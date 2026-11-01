package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"io"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"islet/backend/internal/model"
)

// RepairThumbnailOrientation verifies originals and retains previous previews.
// A false apply performs the same verification without writing storage or rows.
func (s *ImageService) RepairThumbnailOrientation(ctx context.Context, id uint64, apply bool) (bool, error) {
	item, err := s.Get(id)
	if err != nil {
		return false, err
	}
	driver, _, err := s.policies.ReadDriver(item.PolicyID)
	if err != nil {
		return false, err
	}
	r, _, err := driver.Read(ctx, item.StorageKey)
	if err != nil {
		return false, fmt.Errorf("read original %d: %w", id, err)
	}
	data, err := io.ReadAll(io.LimitReader(r, thumbDecodeMax+1))
	r.Close()
	if err != nil {
		return false, err
	}
	if len(data) > thumbDecodeMax {
		return false, nil
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != item.SHA256 {
		return false, fmt.Errorf("original checksum mismatch: %d", id)
	}
	if imageOrientation(data) == 1 {
		return false, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("decode original %d: %w", id, err)
	}
	thumb, ok := buildThumb(data)
	if !ok {
		return false, fmt.Errorf("cannot build preview: %d", id)
	}
	digest := sha256.Sum256(thumb)
	ext, mime := "jpg", "image/jpeg"
	if bytes.HasPrefix(thumb, []byte("\x89PNG\r\n\x1a\n")) {
		ext, mime = "png", "image/png"
	}
	key := fmt.Sprintf("islet-thumbs/%d-oriented-%x.%s", id, digest[:16], ext)
	w, h := orientedSize(cfg.Width, cfg.Height, imageOrientation(data))
	if item.ThumbKey != nil && *item.ThumbKey == key && item.Width == w && item.Height == h {
		return false, nil
	}
	if item.ThumbKey != nil && item.Width == w && item.Height == h {
		if current, _, e := driver.Read(ctx, *item.ThumbKey); e == nil {
			body, e := io.ReadAll(io.LimitReader(current, int64(len(thumb))+1))
			current.Close()
			if e == nil && bytes.Equal(body, thumb) {
				return false, nil
			}
		}
	}
	if !apply {
		return true, nil
	}
	// Existing content-addressed objects are verified before reuse, including
	// recovery after an interrupted write or a database transaction failure.
	if old, _, e := driver.Read(ctx, key); e == nil {
		body, e := io.ReadAll(io.LimitReader(old, int64(len(thumb))+1))
		old.Close()
		if e != nil || !bytes.Equal(body, thumb) {
			return false, fmt.Errorf("preview checksum mismatch: %d", id)
		}
	} else if _, e := driver.Put(ctx, key, bytes.NewReader(thumb), int64(len(thumb)), mime); e != nil {
		return false, e
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.Image
		if e := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, id).Error; e != nil {
			return e
		}
		if current.SHA256 != item.SHA256 || current.StorageKey != item.StorageKey || current.PolicyID != item.PolicyID || stringValue(current.ThumbKey) != stringValue(item.ThumbKey) {
			return fmt.Errorf("image changed during repair: %d", id)
		}
		if e := s.enqueueCDN(tx, current); e != nil {
			return e
		}
		base, _ := s.policies.distribution(current.PolicyID)
		return tx.Unscoped().Model(&current).Updates(map[string]any{"thumb_key": key, "thumb_url": base + "/t/" + current.StorageKey, "width": w, "height": h}).Error
	})
	return err == nil, err
}
