package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"islet/backend/internal/model"
)

var (
	ErrImageNotFound = errors.New("图片不存在")
	ErrAlbumNotFound = errors.New("相册不存在")
)

// ImageService 图库查询、编辑、回收站、彻底删除。
type ImageService struct {
	db        *gorm.DB
	policies  *PolicyService
	cdnClient func(string, string) cdnProvider
}

func NewImageService(db *gorm.DB, policies *PolicyService) *ImageService {
	return &ImageService{db: db, policies: policies}
}

type ImageQuery struct {
	UserID     uint64 // 0=不限（管理员视角）
	AlbumID    *uint64
	Tag        string
	Mime       string
	Keyword    string
	Moderation string
	Recycled   bool
	Before     *time.Time // 游标分页：早于此时间
	BeforeID   uint64
	Limit      int
}

type ImageItem struct {
	ID           uint64     `json:"id"`
	UserID       uint64     `json:"user_id"`
	Username     string     `json:"username,omitempty"`
	PolicyID     uint64     `json:"policy_id"`
	AlbumID      *uint64    `json:"album_id"`
	URL          string     `json:"url"`
	ThumbURL     string     `json:"thumb_url"`
	OriginalName string     `json:"original_name"`
	Mime         string     `json:"mime"`
	Size         uint64     `json:"size"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	Tags         []string   `json:"tags"`
	Blocked      bool       `json:"blocked"`
	BlockReason  string     `json:"block_reason"`
	ModeratedAt  *time.Time `json:"moderated_at,omitempty"`
	CacheSync    string     `json:"cache_sync"`
	CreatedAt    time.Time  `json:"created_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

func (s *ImageService) item(i model.Image, username string) ImageItem {
	base := ""
	if s.policies != nil && s.policies.base != nil {
		base, _ = s.policies.distribution(i.PolicyID)
	}
	return imageView(i, username, base, s.CDNStatus(i.ID))
}

func imageView(i model.Image, username, base, syncState string) ImageItem {
	result := ImageItem{
		ID: i.ID, UserID: i.UserID, Username: username, PolicyID: i.PolicyID,
		AlbumID: i.AlbumID, URL: i.URL, OriginalName: i.OriginalName,
		Mime: i.Mime, Size: i.Size, Width: i.Width, Height: i.Height,
		Tags: i.Tags, CreatedAt: i.CreatedAt,
		Blocked: i.Blocked, BlockReason: i.BlockReason, ModeratedAt: i.ModeratedAt,
	}
	if base != "" {
		result.URL = base + "/i/" + i.StorageKey
		result.ThumbURL = base + "/t/" + i.StorageKey
	} else {
		result.ThumbURL = result.URL
	}
	if i.DeletedAt.Valid {
		t := i.DeletedAt.Time
		result.DeletedAt = &t
	}
	result.CacheSync = syncState
	if result.Tags == nil {
		result.Tags = []string{}
	}
	return result
}

func (s *ImageService) List(q ImageQuery) ([]ImageItem, error) {
	if q.Limit <= 0 || q.Limit > 101 {
		q.Limit = 50
	}
	tx := s.db.Model(&model.Image{})
	if q.Recycled {
		tx = tx.Unscoped().Where("deleted_at IS NOT NULL")
	} else {
		tx = tx.Where("deleted_at IS NULL")
	}
	if q.UserID != 0 {
		tx = tx.Where("images.user_id = ?", q.UserID)
	}
	if q.AlbumID != nil {
		tx = tx.Where("album_id = ?", *q.AlbumID)
	}
	if q.Tag != "" {
		tx = tx.Where("JSON_CONTAINS(tags, ?)", jsonQuote(q.Tag))
	}
	if q.Mime != "" {
		tx = tx.Where("mime LIKE ?", strings.TrimSuffix(q.Mime, "/")+"%")
	}
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("original_name LIKE ? OR url LIKE ?", like, like)
	}
	if q.Moderation == "blocked" {
		tx = tx.Where("blocked = ?", true)
	} else if q.Moderation == "active" {
		tx = tx.Where("blocked = ?", false)
	}
	if q.Before != nil {
		if q.BeforeID > 0 {
			tx = tx.Where("(created_at < ? OR (created_at = ? AND id < ?))", *q.Before, *q.Before, q.BeforeID)
		} else {
			tx = tx.Where("created_at < ?", *q.Before)
		}
	}
	var rows []model.Image
	if err := tx.Order("images.created_at DESC, images.id DESC").Limit(q.Limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	usernames := s.usernameMap(rows)
	ids := make([]uint64, 0, len(rows))
	bases := map[uint64]string{}
	for _, row := range rows {
		ids = append(ids, row.ID)
		if _, ok := bases[row.PolicyID]; !ok {
			base := ""
			if s.policies != nil {
				base, _ = s.policies.distribution(row.PolicyID)
			}
			bases[row.PolicyID] = base
		}
	}
	states := s.cacheStates(ids)
	items := make([]ImageItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, imageView(row, usernames[row.UserID], bases[row.PolicyID], states[row.ID]))
	}
	return items, nil
}

func (s *ImageService) usernameMap(rows []model.Image) map[uint64]string {
	ids := make([]uint64, 0, len(rows))
	seen := map[uint64]bool{}
	for _, r := range rows {
		if !seen[r.UserID] {
			seen[r.UserID] = true
			ids = append(ids, r.UserID)
		}
	}
	if len(ids) == 0 {
		return map[uint64]string{}
	}
	var users []model.User
	if err := s.db.Select("id, username").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return map[uint64]string{}
	}
	result := make(map[uint64]string, len(users))
	for _, u := range users {
		result[u.ID] = u.Username
	}
	return result
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (s *ImageService) Get(id uint64) (model.Image, error) {
	var image model.Image
	err := s.db.Unscoped().First(&image, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return image, ErrImageNotFound
	}
	return image, err
}

// ItemView 上传结果单条视图。
func (s *ImageService) ItemView(i model.Image) ImageItem {
	return s.item(i, "")
}

// GetByKey 按 storage_key 取图片（/i /t 访问路径）。
func (s *ImageService) GetByKey(key string) (model.Image, error) {
	var image model.Image
	err := s.db.Where("storage_key = ?", key).First(&image).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return image, ErrImageNotFound
	}
	return image, err
}

// GetByLegacyKey resolves original and thumbnail paths published before managed links.
func (s *ImageService) GetByLegacyKey(key string) (model.Image, error) {
	var image model.Image
	err := s.db.Where("storage_key = ? OR thumb_key = ?", key, key).First(&image).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return image, ErrImageNotFound
	}
	return image, err
}

type ImageUpdate struct {
	Name    *string   `json:"name"`
	AlbumID *uint64   `json:"album_id"`
	Tags    *[]string `json:"tags"`
}

func (s *ImageService) Update(id, userID uint64, isSuper bool, in ImageUpdate) (model.Image, error) {
	image, err := s.Get(id)
	if err != nil {
		return image, err
	}
	if !isSuper && image.UserID != userID {
		return image, ErrImageNotFound
	}
	updates := map[string]any{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len([]rune(name)) > 255 {
			return image, errors.New("名称不能为空，最多 255 字")
		}
		updates["original_name"] = name
	}
	if in.AlbumID != nil && *in.AlbumID != 0 {
		var album model.Album
		if err := s.db.First(&album, *in.AlbumID).Error; err != nil {
			return image, ErrAlbumNotFound
		}
		if album.UserID != image.UserID {
			return image, ErrAlbumNotFound
		}
		updates["album_id"] = *in.AlbumID
	} else if in.AlbumID != nil {
		updates["album_id"] = nil
	}
	if in.Tags != nil {
		raw, err := json.Marshal(normalizeTags(*in.Tags))
		if err != nil {
			return image, err
		}
		updates["tags"] = string(raw)
	}
	if len(updates) > 0 {
		if err := s.db.Model(&image).Updates(updates).Error; err != nil {
			return image, err
		}
	}
	return s.Get(id)
}

// Recycle 软删除（本人或 super_admin）。
func (s *ImageService) Recycle(id, userID uint64, isSuper bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var image model.Image
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&image, id).Error; err != nil {
			return ErrImageNotFound
		}
		if !isSuper && image.UserID != userID {
			return ErrImageNotFound
		}
		if err := tx.Delete(&image).Error; err != nil {
			return err
		}
		return s.enqueueCDN(tx, image)
	})
}

// Restore 回收站恢复（管理员或本人），恢复时校验分组配额。
func (s *ImageService) Restore(id, userID uint64, isSuper bool, checkQuota func(userID uint64, add uint64) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var image model.Image
		if err := tx.Unscoped().First(&image, id).Error; err != nil {
			return ErrImageNotFound
		}
		if !image.DeletedAt.Valid {
			return errors.New("图片不在回收站")
		}
		if !isSuper && image.UserID != userID {
			return ErrImageNotFound
		}
		var owner model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, image.UserID).Error; err != nil {
			return err
		}
		// Lock the image after its owner so concurrent restores cannot count the same image twice.
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&image, id).Error; err != nil {
			return err
		}
		if !image.DeletedAt.Valid {
			return errors.New("图片已恢复")
		}
		if checkQuota != nil {
			if err := checkQuota(image.UserID, image.Size); err != nil {
				return err
			}
		}
		if err := tx.Unscoped().Model(&image).Update("deleted_at", nil).Error; err != nil {
			return err
		}
		return s.enqueueCDN(tx, image)
	})
}

// Purge 彻底删除：先删存储对象再删记录。
func (s *ImageService) Purge(ctx context.Context, id, userID uint64, isSuper bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var image model.Image
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&image, id).Error; err != nil {
			return ErrImageNotFound
		}
		if !isSuper && image.UserID != userID {
			return ErrImageNotFound
		}
		if !image.DeletedAt.Valid {
			return errors.New("请先将图片移入回收站")
		}
		driver, _, err := s.policies.ReadDriver(image.PolicyID)
		if err != nil {
			return errors.New("暂时无法连接存储，请检查存储策略后重试")
		}
		if err := s.enqueueCDN(tx, image); err != nil {
			return err
		}
		if err := driver.Delete(ctx, image.StorageKey); err != nil {
			return errors.New("原图删除失败，请稍后重试")
		}
		if image.ThumbKey != nil {
			if err := driver.Delete(ctx, *image.ThumbKey); err != nil {
				return errors.New("缩略图删除失败，请稍后重试")
			}
		}
		return tx.Unscoped().Delete(&image).Error
	})
}

// PurgeExpired 清理超过保留期的回收站图片（worker 调用）。
func (s *ImageService) PurgeExpired(ctx context.Context, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	deadline := time.Now().AddDate(0, 0, -retentionDays)
	var rows []model.Image
	if err := s.db.Unscoped().Where("deleted_at IS NOT NULL AND deleted_at < ?", deadline).Limit(200).Find(&rows).Error; err != nil {
		return 0, err
	}
	count := 0
	for _, image := range rows {
		if err := s.Purge(ctx, image.ID, image.UserID, true); err == nil {
			count++
		}
	}
	return count, nil
}

// ---- 相册 ----

type AlbumService struct {
	db *gorm.DB
}

func NewAlbumService(db *gorm.DB) *AlbumService { return &AlbumService{db: db} }

type AlbumInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *AlbumService) List(userID uint64) ([]model.Album, error) {
	var albums []model.Album
	err := s.db.Where("user_id = ?", userID).Order("id ASC").Find(&albums).Error
	return albums, err
}

func (s *AlbumService) Create(userID uint64, in AlbumInput) (model.Album, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 64 {
		return model.Album{}, errors.New("相册名称不能为空，最多 64 字")
	}
	album := model.Album{UserID: userID, Name: name, Description: strings.TrimSpace(in.Description)}
	if err := s.db.Create(&album).Error; err != nil {
		return model.Album{}, err
	}
	return album, nil
}

func (s *AlbumService) Update(userID uint64, id uint64, in AlbumInput) (model.Album, error) {
	var album model.Album
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&album).Error; err != nil {
		return model.Album{}, ErrAlbumNotFound
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 64 {
		return model.Album{}, errors.New("相册名称不能为空，最多 64 字")
	}
	album.Name = name
	album.Description = strings.TrimSpace(in.Description)
	if err := s.db.Save(&album).Error; err != nil {
		return model.Album{}, err
	}
	return album, nil
}

// Delete 空相册才能删（防图片悬空）。
func (s *AlbumService) Delete(userID uint64, id uint64) error {
	var album model.Album
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&album).Error; err != nil {
		return ErrAlbumNotFound
	}
	var count int64
	if err := s.db.Unscoped().Model(&model.Image{}).Where("album_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("相册内仍有图片（含回收站），请先移出或清理")
	}
	return s.db.Delete(&album).Error
}
