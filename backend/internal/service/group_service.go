package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"islet/backend/internal/model"
)

var (
	ErrGroupNotFound   = errors.New("用户分组不存在")
	ErrGroupNameExists = errors.New("分组名称已存在")
	ErrGroupBuiltIn    = errors.New("内置分组不可删除")
	ErrGroupInUse      = errors.New("分组仍有用户，无法删除")
)

// GroupService 用户分组管理。
type GroupService struct {
	db *gorm.DB
}

func NewGroupService(db *gorm.DB) *GroupService { return &GroupService{db: db} }

type GroupInput struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	QuotaBytes     uint64   `json:"quota_bytes"`
	MaxFileBytes   uint64   `json:"max_file_bytes"`
	AllowedMimes   []string `json:"allowed_mimes"`
	PolicyIDs      []uint64 `json:"policy_ids"`
	UploadsPerHour int      `json:"uploads_per_hour"`
	Status         string   `json:"status"`
}

func normalizeMimes(mimes []string) []string {
	result := make([]string, 0, len(mimes))
	for _, m := range mimes {
		m = strings.ToLower(strings.TrimSpace(m))
		if m == "" {
			continue
		}
		if m != "image/*" && !strings.HasPrefix(m, "image/") {
			continue
		}
		result = append(result, m)
	}
	return result
}

func (s *GroupService) List() ([]model.UserGroup, error) {
	var groups []model.UserGroup
	err := s.db.Order("id ASC").Find(&groups).Error
	return groups, err
}

func (s *GroupService) Get(id uint64) (model.UserGroup, error) {
	var group model.UserGroup
	err := s.db.First(&group, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return group, ErrGroupNotFound
	}
	return group, err
}

func (s *GroupService) Create(in GroupInput) (model.UserGroup, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 64 {
		return model.UserGroup{}, errors.New("分组名称不能为空，最多 64 字")
	}
	group := model.UserGroup{
		Name: name, Description: strings.TrimSpace(in.Description),
		QuotaBytes: in.QuotaBytes, MaxFileBytes: in.MaxFileBytes,
		AllowedMimes: normalizeMimes(in.AllowedMimes), PolicyIDs: in.PolicyIDs,
		UploadsPerHour: in.UploadsPerHour,
		Status:         model.StatusActive,
	}
	if in.Status == model.StatusDisabled {
		group.Status = model.StatusDisabled
	}
	if err := s.db.Create(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.UserGroup{}, ErrGroupNameExists
		}
		return model.UserGroup{}, err
	}
	return group, nil
}

func (s *GroupService) Update(id uint64, in GroupInput) (model.UserGroup, error) {
	var group model.UserGroup
	if err := s.db.First(&group, id).Error; err != nil {
		return model.UserGroup{}, ErrGroupNotFound
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 64 {
		return model.UserGroup{}, errors.New("分组名称不能为空，最多 64 字")
	}
	updates := map[string]any{
		"name": name, "description": strings.TrimSpace(in.Description),
		"quota_bytes": in.QuotaBytes, "max_file_bytes": in.MaxFileBytes,
		"allowed_mimes": normalizeMimes(in.AllowedMimes), "policy_ids": in.PolicyIDs,
		"uploads_per_hour": in.UploadsPerHour,
	}
	if in.Status == model.StatusActive || in.Status == model.StatusDisabled {
		updates["status"] = in.Status
	}
	if err := s.db.Model(&group).Updates(updates).Error; err != nil {
		return model.UserGroup{}, err
	}
	return s.Get(id)
}

func (s *GroupService) Delete(id uint64) error {
	var group model.UserGroup
	if err := s.db.First(&group, id).Error; err != nil {
		return ErrGroupNotFound
	}
	if group.BuiltIn {
		return ErrGroupBuiltIn
	}
	var count int64
	if err := s.db.Model(&model.User{}).Where("group_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrGroupInUse
	}
	return s.db.Delete(&group).Error
}

//mimeAllowed 校验 mime 是否命中白名单（支持 image/* 通配；空白名单=不限）。
func mimeAllowed(allowed []string, mime string) bool {
	if len(allowed) == 0 {
		return strings.HasPrefix(mime, "image/")
	}
	for _, pattern := range allowed {
		if pattern == "image/*" {
			if strings.HasPrefix(mime, "image/") {
				return true
			}
			continue
		}
		if pattern == mime {
			return true
		}
	}
	return false
}
