package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"islet/backend/internal/model"
)

// StatsService 统计看板：总量、趋势、按策略/分组分布、回收站。
type StatsService struct {
	db       *gorm.DB
	uploads  *UploadService
}

func NewStatsService(db *gorm.DB, uploads *UploadService) *StatsService {
	return &StatsService{db: db, uploads: uploads}
}

type Overview struct {
	TotalImages    uint64            `json:"total_images"`
	TotalBytes     uint64            `json:"total_bytes"`
	TotalUsers     uint64            `json:"total_users"`
	PendingUsers   uint64            `json:"pending_users"`
	TodayImages    uint64            `json:"today_images"`
	RecycledCount  uint64            `json:"recycled_count"`
	StorageUsage   map[string]uint64 `json:"storage_usage"`
	GroupCount     uint64            `json:"group_count"`
	PolicyCount    uint64            `json:"policy_count"`
}

func (s *StatsService) Overview() (Overview, error) {
	var result Overview
	today := time.Now().Format("2006-01-02")
	queries := []struct {
		dest   *uint64
		query  string
	}{
		{&result.TotalImages, "SELECT COUNT(*) FROM images WHERE deleted_at IS NULL"},
		{&result.TotalBytes, "SELECT COALESCE(SUM(size),0) FROM images WHERE deleted_at IS NULL"},
		{&result.TotalUsers, "SELECT COUNT(*) FROM users WHERE deleted_at IS NULL"},
		{&result.PendingUsers, "SELECT COUNT(*) FROM users WHERE status = 'pending' AND deleted_at IS NULL"},
		{&result.TodayImages, "SELECT COUNT(*) FROM images WHERE deleted_at IS NULL AND DATE(created_at) = ?"},
		{&result.RecycledCount, "SELECT COUNT(*) FROM images WHERE deleted_at IS NOT NULL"},
		{&result.GroupCount, "SELECT COUNT(*) FROM user_groups"},
		{&result.PolicyCount, "SELECT COUNT(*) FROM storage_policies WHERE deleted_at IS NULL"},
	}
	for _, q := range queries {
		args := []any{}
		if q.dest == &result.TodayImages {
			args = append(args, today)
		}
		if err := s.db.Raw(q.query, args...).Scan(q.dest).Error; err != nil {
			return result, err
		}
	}
	result.StorageUsage = map[string]uint64{}
	var byPolicy []struct {
		PolicyID uint64
		Total    uint64
	}
	if err := s.db.Model(&model.Image{}).Select("policy_id, COALESCE(SUM(size),0) AS total").
		Where("deleted_at IS NULL").Group("policy_id").Scan(&byPolicy).Error; err == nil {
		for _, row := range byPolicy {
			result.StorageUsage[uint64ToString(row.PolicyID)] = row.Total
		}
	}
	return result, nil
}

func uint64ToString(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

type TrendPoint struct {
	Date   string `json:"date"`
	Count  uint64 `json:"count"`
	Bytes  uint64 `json:"bytes"`
}

// Trend 近 N 天上传趋势（含空天补零）。
func (s *StatsService) Trend(days int) ([]TrendPoint, error) {
	if days <= 0 || days > 90 {
		days = 14
	}
	start := time.Now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	var rows []TrendPoint
	if err := s.db.Raw(
		"SELECT DATE_FORMAT(created_at, '%Y-%m-%d') AS date, COUNT(*) AS count, COALESCE(SUM(size),0) AS bytes "+
			"FROM images WHERE deleted_at IS NULL AND DATE(created_at) >= ? GROUP BY DATE_FORMAT(created_at, '%Y-%m-%d') ORDER BY date", start,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	byDate := map[string]TrendPoint{}
	for _, row := range rows {
		byDate[row.Date] = row
	}
	result := make([]TrendPoint, 0, days)
	for i := days - 1; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		if point, ok := byDate[date]; ok {
			result = append(result, point)
		} else {
			result = append(result, TrendPoint{Date: date})
		}
	}
	return result, nil
}

type GroupUsage struct {
	GroupID    uint64 `json:"group_id"`
	Name       string `json:"name"`
	QuotaBytes uint64 `json:"quota_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
	Images     uint64 `json:"images"`
}

func (s *StatsService) GroupUsage() ([]GroupUsage, error) {
	var groups []model.UserGroup
	if err := s.db.Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	result := make([]GroupUsage, 0, len(groups))
	for _, g := range groups {
		var used, images uint64
		if err := s.db.Raw("SELECT COALESCE(SUM(size),0) FROM images i JOIN users u ON u.id = i.user_id "+
			"WHERE u.group_id = ? AND i.deleted_at IS NULL", g.ID).Scan(&used).Error; err != nil {
			return nil, err
		}
		if err := s.db.Raw("SELECT COUNT(*) FROM images i JOIN users u ON u.id = i.user_id "+
			"WHERE u.group_id = ? AND i.deleted_at IS NULL", g.ID).Scan(&images).Error; err != nil {
			return nil, err
		}
		result = append(result, GroupUsage{
			GroupID: g.ID, Name: g.Name, QuotaBytes: g.QuotaBytes, UsedBytes: used, Images: images,
		})
	}
	return result, nil
}

// ---- 审计 ----

type AuditService struct {
	db *gorm.DB
}

func NewAuditService(db *gorm.DB) *AuditService { return &AuditService{db: db} }

func (s *AuditService) Record(userID *uint64, action, targetType, targetID string, detail any, ip string) {
	entry := model.AuditLog{Action: action}
	if userID != nil {
		entry.UserID = userID
	}
	if targetType != "" {
		entry.TargetType = &targetType
	}
	if targetID != "" {
		entry.TargetID = &targetID
	}
	if detail != nil {
		if raw, err := json.Marshal(detail); err == nil {
			entry.Detail = raw
		}
	}
	if ip != "" {
		entry.IP = &ip
	}
	_ = s.db.Create(&entry).Error
}

type AuditItem struct {
	ID         uint64     `json:"id"`
	UserID     *uint64    `json:"user_id"`
	Username   string     `json:"username,omitempty"`
	Action     string     `json:"action"`
	TargetType string     `json:"target_type,omitempty"`
	TargetID   string     `json:"target_id,omitempty"`
	Detail     string     `json:"detail,omitempty"`
	IP         string     `json:"ip,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (s *AuditService) List(before *time.Time, limit int) ([]AuditItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tx := s.db.Model(&model.AuditLog{})
	if before != nil {
		tx = tx.Where("created_at < ?", *before)
	}
	var rows []model.AuditLog
	if err := tx.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	userIDs := map[uint64]bool{}
	for _, r := range rows {
		if r.UserID != nil {
			userIDs[*r.UserID] = true
		}
	}
	usernames := map[uint64]string{}
	if len(userIDs) > 0 {
		ids := make([]uint64, 0, len(userIDs))
		for id := range userIDs {
			ids = append(ids, id)
		}
		var users []model.User
		if err := s.db.Select("id, username").Where("id IN ?", ids).Find(&users).Error; err == nil {
			for _, u := range users {
				usernames[u.ID] = u.Username
			}
		}
	}
	items := make([]AuditItem, 0, len(rows))
	for _, r := range rows {
		item := AuditItem{ID: r.ID, UserID: r.UserID, Action: r.Action, CreatedAt: r.CreatedAt}
		item.Username = usernames[ derefUint64(r.UserID) ]
		if r.TargetType != nil {
			item.TargetType = *r.TargetType
		}
		if r.TargetID != nil {
			item.TargetID = *r.TargetID
		}
		if r.Detail != nil {
			item.Detail = string(r.Detail)
		}
		if r.IP != nil {
			item.IP = *r.IP
		}
		items = append(items, item)
	}
	return items, nil
}

func derefUint64(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}

// PurgeAllRecycle 管理员清空回收站。
func (s *ImageService) PurgeAllRecycle(ctx context.Context) (int, error) {
	var rows []model.Image
	if err := s.db.Unscoped().Where("deleted_at IS NOT NULL").Limit(500).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, errors.New("回收站为空")
	}
	count := 0
	for _, image := range rows {
		if err := s.Purge(ctx, image.ID, image.UserID, true); err == nil {
			count++
		}
	}
	return count, nil
}

// RecycleList 管理员回收站总览。
func (s *ImageService) RecycleList(limit int) ([]ImageItem, error) {
	return s.List(ImageQuery{Recycled: true, Limit: limit})
}
