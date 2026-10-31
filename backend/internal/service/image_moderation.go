package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"islet/backend/internal/model"
)

var ErrModerationReason = errors.New("请填写违规原因，最多 500 字")

// Moderate preserves the stored object and commits the decision and its audit together.
func (s *ImageService) Moderate(id, moderator uint64, blocked bool, reason, ip string) (model.Image, error) {
	reason = strings.TrimSpace(reason)
	if blocked && (reason == "" || len([]rune(reason)) > 500) {
		return model.Image{}, ErrModerationReason
	}
	if !blocked {
		reason = ""
	}
	var image model.Image
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&image, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrImageNotFound
			}
			return err
		}
		now := time.Now()
		previous := image.Blocked
		if err := tx.Model(&image).Updates(map[string]any{"blocked": blocked, "block_reason": reason, "moderated_by": moderator, "moderated_at": now}).Error; err != nil {
			return err
		}
		action, target, targetID := "image.allow", "image", strconv.FormatUint(id, 10)
		if blocked {
			action = "image.block"
		}
		detail, err := json.Marshal(map[string]any{"previous_blocked": previous, "blocked": blocked, "reason": reason})
		if err != nil {
			return err
		}
		entry := model.AuditLog{UserID: &moderator, Action: action, TargetType: &target, TargetID: &targetID, Detail: detail, IP: &ip}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		return s.enqueueCDN(tx, image)
	})
	return image, err
}
