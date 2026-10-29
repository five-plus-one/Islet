// importlsky imports a protected Lsky export into an initialized Islet instance.
package main

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"
	"islet/backend/internal/config"
	"islet/backend/internal/database"
	"islet/backend/internal/model"
	"islet/backend/internal/service"
)

type source struct {
	Policy     service.PolicyInput `json:"policy"`
	Images     []legacyImage       `json:"images"`
	UserCount  int                 `json:"user_count"`
	AlbumCount int                 `json:"album_count"`
}
type legacyImage struct {
	Key     string `json:"key"`
	URL     string `json:"url"`
	Name    string `json:"name"`
	Mime    string `json:"mime"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Created string `json:"created"`
	SHA1    string `json:"sha1"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Import failed:", err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("source", "", "Protected source JSON")
	owner := flag.Uint64("owner", 0, "Existing destination owner ID")
	apply := flag.Bool("apply", false, "Apply validated import")
	flag.Parse()
	if *path == "" || *owner == 0 {
		return errors.New("source and owner are required")
	}
	raw, err := os.ReadFile(*path)
	if err != nil {
		return errors.New("cannot read protected export")
	}
	var src source
	if json.Unmarshal(raw, &src) != nil {
		return errors.New("invalid export")
	}
	if src.UserCount != 1 || src.AlbumCount != 0 {
		return errors.New("this importer requires one owner and no source albums")
	}
	if len(src.Images) == 0 {
		return errors.New("source contains no images")
	}
	base, err := url.Parse(src.Policy.CDN)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return errors.New("source CDN must use HTTPS")
	}
	cfg := config.Load()
	db, err := database.ConnectMySQL(cfg.DSN())
	if err != nil {
		return errors.New("destination database unavailable")
	}
	var user model.User
	if db.First(&user, *owner).Error != nil {
		return errors.New("destination owner not found")
	}
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != base.Host || req.URL.Scheme != "https" || len(via) > 3 {
			return errors.New("unexpected image redirect")
		}
		return nil
	}}
	images := make([]model.Image, 0, len(src.Images))
	var total uint64
	for i, old := range src.Images {
		u, e := url.Parse(old.URL)
		if e != nil || u.Host != base.Host || u.Scheme != "https" || old.Key == "" || strings.Contains(old.Key, "..") || strings.HasPrefix(old.Key, "/") {
			return fmt.Errorf("invalid image metadata at index %d", i)
		}
		resp, e := client.Get(old.URL)
		if e != nil {
			return fmt.Errorf("image %d could not be read", i)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return fmt.Errorf("image %d returned HTTP %d", i, resp.StatusCode)
		}
		data, e := io.ReadAll(io.LimitReader(resp.Body, 100<<20+1))
		resp.Body.Close()
		if e != nil || len(data) > 100<<20 {
			return fmt.Errorf("image %d exceeds read limits", i)
		}
		originalHash := sha1.Sum(data)
		if old.SHA1 != "" && !strings.EqualFold(hex.EncodeToString(originalHash[:]), old.SHA1) {
			return fmt.Errorf("source checksum differs at index %d", i)
		}
		hash := sha256.Sum256(data)
		created, e := time.Parse(time.RFC3339, old.Created)
		if e != nil {
			return fmt.Errorf("invalid date at index %d", i)
		}
		images = append(images, model.Image{UserID: *owner, StorageKey: old.Key, URL: old.URL, OriginalName: old.Name, Mime: old.Mime, Size: uint64(len(data)), Width: old.Width, Height: old.Height, SHA256: hex.EncodeToString(hash[:]), Tags: []string{}, CreatedAt: created, UpdatedAt: created})
		total += uint64(len(data))
	}
	if !*apply {
		fmt.Printf("Validated %d images, %d bytes; destination unchanged\n", len(images), total)
		return nil
	}
	inserted := 0
	err = db.Transaction(func(tx *gorm.DB) error {
		var policy model.StoragePolicy
		if e := tx.Where("name = ?", src.Policy.Name).First(&policy).Error; e != nil {
			if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			p, e := service.NewPolicyService(tx, service.NewSecretBox(cfg.DataDir), func() string { return "https://img.five-plus-one.com" }, cfg.DataDir).Create(src.Policy)
			if e != nil {
				return errors.New("source policy could not be imported")
			}
			policy.ID = p.ID
		}
		for _, image := range images {
			var existing model.Image
			e := tx.Unscoped().Where("storage_key = ?", image.StorageKey).First(&existing).Error
			if e == nil {
				if existing.URL != image.URL || existing.SHA256 != image.SHA256 || existing.UserID != image.UserID {
					return errors.New("existing image conflicts with source")
				}
				continue
			}
			if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			image.PolicyID = policy.ID
			if e = tx.Create(&image).Error; e != nil {
				return errors.New("could not save imported image")
			}
			inserted++
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("Imported %d images; validated %d images and %d bytes\n", inserted, len(images), total)
	return nil
}
