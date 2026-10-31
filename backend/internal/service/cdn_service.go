package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"islet/backend/internal/model"
)

type cdnProvider interface {
	refresh(context.Context, []string) (string, error)
	complete(context.Context, string, []string) (bool, error)
}

func (s *PolicyService) distribution(id uint64) (string, bool) {
	base := strings.TrimRight(s.base(), "/")
	var policy model.StoragePolicy
	if s.db.First(&policy, id).Error != nil || policy.Driver != model.PolicyDriverS3 {
		return base, false
	}
	payload, err := s.decryptConfig(policy)
	if err != nil || payload.CDN == "" || !payload.CDNRefresh {
		return base, false
	}
	return strings.TrimRight(payload.CDN, "/"), true
}

func (s *ImageService) CDNCacheEnabled(id uint64) bool {
	_, enabled := s.policies.distribution(id)
	return enabled
}

func (s *ImageService) enqueueCDN(tx *gorm.DB, image model.Image) error {
	if s.policies == nil {
		return nil
	}
	var policy model.StoragePolicy
	if err := tx.First(&policy, image.PolicyID).Error; err != nil {
		return err
	}
	if policy.Driver != model.PolicyDriverS3 {
		return nil
	}
	payload, err := s.policies.decryptConfig(policy)
	if err != nil {
		return err
	}
	if payload.CDN == "" || !payload.CDNRefresh {
		return nil
	}
	base := strings.TrimRight(payload.CDN, "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return errors.New("图片加速地址不可用")
	}
	// Include both published protocols and every managed legacy path. Query strings
	// are excluded by the CDN cache-key configuration.
	paths := []string{"/i/" + image.StorageKey, "/t/" + image.StorageKey, "/" + image.StorageKey, "/legacy/" + image.StorageKey}
	if image.ThumbKey != nil {
		paths = append(paths, "/"+*image.ThumbKey, "/legacy/"+*image.ThumbKey)
	}
	for _, stored := range []string{image.URL, stringValue(image.ThumbURL)} {
		if old, e := url.Parse(stored); e == nil && old.Host == parsed.Host {
			paths = append(paths, old.Path)
		}
	}
	urls, seen := []string{}, map[string]bool{}
	for _, path := range paths {
		for _, scheme := range []string{"https", "http"} {
			target := (&url.URL{Scheme: scheme, Host: parsed.Host, Path: strings.TrimRight(parsed.Path, "/") + path}).String()
			if !seen[target] {
				seen[target] = true
				urls = append(urls, target)
			}
		}
	}
	return tx.Create(&model.CDNCacheJob{ImageID: image.ID, PolicyID: image.PolicyID, URLs: urls, Status: "pending", NextAttemptAt: time.Now()}).Error
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SyncCDN claims a short lease before making network calls. A crashed worker's
// lease expires, so another instance can finish the persisted task.
func (s *ImageService) SyncCDN(ctx context.Context) error {
	for n := 0; n < 3; n++ {
		var job model.CDNCacheJob
		err := s.db.Transaction(func(tx *gorm.DB) error {
			result := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status <> ? AND next_attempt_at <= ?", "complete", time.Now()).Order("next_attempt_at, id").Limit(1).Find(&job)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			job.NextAttemptAt = time.Now().Truncate(time.Millisecond).Add(2 * time.Minute)
			return tx.Model(&job).Update("next_attempt_at", job.NextAttemptAt).Error
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		jobCtx, cancel := context.WithTimeout(ctx, 70*time.Second)
		state, taskID, code := s.processCDN(jobCtx, job)
		cancel()
		updates := map[string]any{"status": state, "task_ids": taskID, "last_error": code, "next_attempt_at": time.Now().Add(30 * time.Second)}
		if taskID != "" && job.TaskIDs == "" {
			updates["submitted_at"] = time.Now()
		}
		if code != "" {
			attempts := job.Attempts + 1
			delay := time.Minute * time.Duration(1<<min(attempts-1, 6))
			updates["attempts"] = attempts
			updates["next_attempt_at"] = time.Now().Add(delay)
		}
		if err := s.db.Model(&model.CDNCacheJob{}).Where("id = ? AND next_attempt_at = ? AND status <> ?", job.ID, job.NextAttemptAt, "complete").Updates(updates).Error; err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func (s *ImageService) processCDN(ctx context.Context, job model.CDNCacheJob) (string, string, string) {
	var policy model.StoragePolicy
	if s.db.First(&policy, job.PolicyID).Error != nil {
		return job.Status, job.TaskIDs, "PolicyUnavailable"
	}
	payload, err := s.policies.decryptConfig(policy)
	if err != nil {
		return job.Status, job.TaskIDs, "CredentialUnavailable"
	}
	var client cdnProvider = newAliyunCDN(payload.AccessKey, payload.SecretKey)
	if s.cdnClient != nil {
		client = s.cdnClient(payload.AccessKey, payload.SecretKey)
	}
	if job.TaskIDs == "" {
		id, err := client.refresh(ctx, job.URLs)
		if err != nil {
			return "pending", "", cdnErrorCode(err)
		}
		return "submitted", id, ""
	}
	complete, err := client.complete(ctx, job.TaskIDs, job.URLs)
	if err != nil {
		if cdnErrorCode(err) == "RefreshFailed" || (job.SubmittedAt != nil && time.Since(*job.SubmittedAt) > 48*time.Hour) {
			return "pending", "", cdnErrorCode(err)
		}
		return "submitted", job.TaskIDs, cdnErrorCode(err)
	}
	if complete {
		return "complete", job.TaskIDs, ""
	}
	// Resubmit tasks that aged out of the provider's three-day query window.
	if job.SubmittedAt != nil && time.Since(*job.SubmittedAt) > 48*time.Hour {
		return "pending", "", "RefreshExpired"
	}
	return "submitted", job.TaskIDs, ""
}

func (s *ImageService) CDNStatus(id uint64) string {
	return s.cacheStates([]uint64{id})[id]
}

func (s *ImageService) cacheStates(ids []uint64) map[uint64]string {
	states := map[uint64]string{}
	if len(ids) == 0 {
		return states
	}
	var jobs []model.CDNCacheJob
	err := s.db.Select("image_id, last_error").Where("image_id IN ? AND status <> ?", ids, "complete").Find(&jobs).Error
	for _, id := range ids {
		states[id] = "complete"
		if err != nil {
			states[id] = "pending"
		}
	}
	for _, job := range jobs {
		if job.LastError != "" {
			states[job.ImageID] = "retrying"
		} else if states[job.ImageID] != "retrying" {
			states[job.ImageID] = "pending"
		}
	}
	return states
}

func (s *ImageService) VisibleCacheStates(ids []uint64, userID uint64, admin bool) (map[uint64]string, error) {
	var visible []uint64
	query := s.db.Unscoped().Model(&model.Image{}).Where("id IN ?", ids)
	if !admin {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Pluck("id", &visible).Error; err != nil {
		return nil, err
	}
	return s.cacheStates(visible), nil
}
