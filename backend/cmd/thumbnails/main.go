// thumbnails generates verified previews for an imported owner's image library.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"islet/backend/internal/config"
	"islet/backend/internal/database"
	"islet/backend/internal/model"
	"islet/backend/internal/service"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Preview generation failed:", err)
		os.Exit(1)
	}
}
func run() error {
	owner := flag.Uint64("owner", 0, "Imported owner ID")
	flag.Parse()
	if *owner == 0 {
		return fmt.Errorf("owner is required")
	}
	cfg := config.Load()
	db, err := database.ConnectMySQL(cfg.DSN())
	if err != nil {
		return fmt.Errorf("database unavailable")
	}
	policies := service.NewPolicyService(db, service.NewSecretBox(cfg.DataDir), func() string { return cfg.AdminBaseURL }, cfg.DataDir)
	var images []model.Image
	if err = db.Where("user_id = ? AND thumb_key IS NULL", *owner).Find(&images).Error; err != nil {
		return err
	}
	total := 0
	for _, item := range images {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		driver, _, e := policies.Driver(item.PolicyID)
		if e != nil {
			cancel()
			return e
		}
		reader, _, e := driver.Read(ctx, item.StorageKey)
		if e != nil {
			cancel()
			return fmt.Errorf("cannot read image %d", item.ID)
		}
		data, e := io.ReadAll(io.LimitReader(reader, 32<<20+1))
		reader.Close()
		if e != nil || len(data) > 32<<20 {
			cancel()
			return fmt.Errorf("image %d exceeds preview limit", item.ID)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != item.SHA256 {
			cancel()
			return fmt.Errorf("checksum differs for image %d", item.ID)
		}
		thumb, ok := service.BuildThumbnail(data)
		if !ok {
			cancel()
			return fmt.Errorf("cannot decode image %d", item.ID)
		}
		extension, mime := "jpg", "image/jpeg"
		if bytes.HasPrefix(thumb, []byte{137, 80, 78, 71}) {
			extension, mime = "png", "image/png"
		}
		key := fmt.Sprintf("islet-thumbs/%d-%s.%s", item.ID, item.SHA256[:16], extension)
		url, e := driver.Put(ctx, key, bytes.NewReader(thumb), int64(len(thumb)), mime)
		cancel()
		if e != nil {
			return fmt.Errorf("cannot save preview %d: %w", item.ID, e)
		}
		if e = db.Model(&item).Updates(map[string]any{"thumb_key": key, "thumb_url": url}).Error; e != nil {
			return e
		}
		total += len(thumb)
	}
	fmt.Printf("Generated %d previews, %d bytes\n", len(images), total)
	return nil
}
