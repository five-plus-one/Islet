package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"islet/backend/internal/model"
	"islet/backend/internal/storage"
)

var (
	ErrPolicyNotFound   = errors.New("存储策略不存在")
	ErrPolicyNameExists = errors.New("策略名称已存在")
	ErrPolicyInUse      = errors.New("策略仍有图片引用，无法删除")
)

// PolicyService 存储策略管理：密文配置、驱动构造、连通测试。
type PolicyService struct {
	db      *gorm.DB
	box     *SecretBox
	base    func() string
	dataDir string
}

func NewPolicyService(db *gorm.DB, box *SecretBox, baseURL func() string, dataDir string) *PolicyService {
	return &PolicyService{db: db, box: box, base: baseURL, dataDir: dataDir}
}

// PolicyView API 回传视图（永不回传 Secret）。
type PolicyView struct {
	ID              uint64 `json:"id"`
	Name            string `json:"name"`
	Driver          string `json:"driver"`
	URLMode         string `json:"url_mode"`
	Naming          string `json:"naming"`
	IsDefault       bool   `json:"is_default"`
	Status          string `json:"status"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket,omitempty"`
	Prefix          string `json:"prefix,omitempty"`
	CDN             string `json:"cdn,omitempty"`
	AccessKey       string `json:"access_key,omitempty"`
	PathStyle       bool   `json:"path_style,omitempty"`
	SecretConfigured bool  `json:"secret_configured"`
}

type PolicyInput struct {
	ID        uint64  `json:"id"`
	Name      string  `json:"name"`
	Driver    string  `json:"driver"`
	URLMode   string  `json:"url_mode"`
	Naming    string  `json:"naming"`
	IsDefault bool    `json:"is_default"`
	Status    string  `json:"status"`
	Endpoint  string  `json:"endpoint"`
	Region    string  `json:"region"`
	Bucket    string  `json:"bucket"`
	Prefix    string  `json:"prefix"`
	CDN       string  `json:"cdn"`
	AccessKey string  `json:"access_key"`
	SecretKey string  `json:"secret_key"`
	PathStyle bool    `json:"path_style"`
}

func (s *PolicyService) decryptConfig(policy model.StoragePolicy) (S3Payload, error) {
	var payload S3Payload
	plain, err := s.box.Open(policy.Config)
	if err != nil {
		return payload, err
	}
	if err := json.Unmarshal([]byte(plain), &payload); err != nil {
		return payload, ErrSecretUndecryptable
	}
	return payload, nil
}

// S3Payload 策略密文中的 S3 配置；local 策略存空对象。
type S3Payload struct {
	storage.S3Config
}

func (s *PolicyService) List() ([]PolicyView, error) {
	var policies []model.StoragePolicy
	if err := s.db.Order("id ASC").Find(&policies).Error; err != nil {
		return nil, err
	}
	views := make([]PolicyView, 0, len(policies))
	for _, p := range policies {
		v, err := s.view(p)
		if err != nil {
			// 解不开的策略也展示基础信息
			views = append(views, PolicyView{ID: p.ID, Name: p.Name, Driver: p.Driver, URLMode: p.URLMode, Naming: p.Naming, IsDefault: p.IsDefault, Status: p.Status})
			continue
		}
		views = append(views, v)
	}
	return views, nil
}

func (s *PolicyService) view(p model.StoragePolicy) (PolicyView, error) {
	result := PolicyView{
		ID: p.ID, Name: p.Name, Driver: p.Driver, URLMode: p.URLMode,
		Naming: p.Naming, IsDefault: p.IsDefault, Status: p.Status,
	}
	if p.Driver == model.PolicyDriverS3 {
		payload, err := s.decryptConfig(p)
		if err != nil {
			return result, err
		}
		result.Endpoint = payload.Endpoint
		result.Region = payload.Region
		result.Bucket = payload.Bucket
		result.Prefix = payload.Prefix
		result.CDN = payload.CDN
		result.AccessKey = payload.AccessKey
		result.PathStyle = payload.PathStyle
		result.SecretConfigured = payload.SecretKey != ""
	}
	return result, nil
}

func (s *PolicyService) sealInput(in PolicyInput) (string, error) {
	payload := S3Payload{S3Config: storage.S3Config{
		Endpoint: strings.TrimSpace(in.Endpoint), Region: strings.TrimSpace(in.Region),
		Bucket: strings.TrimSpace(in.Bucket), Prefix: strings.TrimSpace(in.Prefix),
		CDN: strings.TrimSpace(in.CDN), AccessKey: strings.TrimSpace(in.AccessKey),
		SecretKey: in.SecretKey, PathStyle: in.PathStyle,
	}}
	if err := storage.ValidateS3(payload.S3Config); err != nil {
		return "", err
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return s.box.Seal(string(plain))
}

// Create 新建策略。secret 为空且 driver=s3 时报错。
func (s *PolicyService) Create(in PolicyInput) (PolicyView, error) {
	if strings.TrimSpace(in.Name) == "" || len([]rune(in.Name)) > 64 {
		return PolicyView{}, errors.New("策略名称不能为空，最多 64 字")
	}
	in.Driver = strings.TrimSpace(in.Driver)
	if in.Driver != model.PolicyDriverLocal && in.Driver != model.PolicyDriverS3 {
		return PolicyView{}, errors.New("driver 须为 local 或 s3")
	}
	if in.Naming != model.NamingKeep && in.Naming != model.NamingRandom && in.Naming != model.NamingDate {
		in.Naming = model.NamingDate
	}
	if in.URLMode != model.PolicyURLDirect && in.URLMode != model.PolicyURLProxy {
		in.URLMode = model.PolicyURLDirect
	}
	if in.Status == "" {
		in.Status = model.StatusActive
	}
	var sealed string
	if in.Driver == model.PolicyDriverS3 {
		var err error
		if sealed, err = s.sealInput(in); err != nil {
			return PolicyView{}, err
		}
	} else {
		sealed = "{}"
	}
	policy := model.StoragePolicy{
		Name: strings.TrimSpace(in.Name), Driver: in.Driver, Config: sealed,
		URLMode: in.URLMode, Naming: in.Naming, IsDefault: in.IsDefault, Status: in.Status,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if in.IsDefault {
			if err := tx.Model(&model.StoragePolicy{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&policy).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "Duplicate") {
				return ErrPolicyNameExists
			}
			return err
		}
		// 无默认策略时自动设为默认
		var count int64
		if err := tx.Model(&model.StoragePolicy{}).Where("is_default = ? AND deleted_at IS NULL", true).Count(&count).Error; err == nil && count == 0 {
			return tx.Model(&policy).Update("is_default", true).Error
		}
		return nil
	})
	if err != nil {
		return PolicyView{}, err
	}
	return s.view(policy)
}

// Update 更新策略。SecretKey 为空时保留旧值；旧密文解不开时必须填写新 Secret。
func (s *PolicyService) Update(id uint64, in PolicyInput) (PolicyView, error) {
	var policy model.StoragePolicy
	if err := s.db.First(&policy, id).Error; err != nil {
		return PolicyView{}, ErrPolicyNotFound
	}
	if strings.TrimSpace(in.Name) == "" || len([]rune(in.Name)) > 64 {
		return PolicyView{}, errors.New("策略名称不能为空，最多 64 字")
	}
	if in.Naming != model.NamingKeep && in.Naming != model.NamingRandom && in.Naming != model.NamingDate {
		in.Naming = policy.Naming
	}
	if in.URLMode != model.PolicyURLDirect && in.URLMode != model.PolicyURLProxy {
		in.URLMode = policy.URLMode
	}
	driver := policy.Driver
	if in.Driver == model.PolicyDriverLocal || in.Driver == model.PolicyDriverS3 {
		driver = in.Driver
	}
	sealed := policy.Config
	if driver == model.PolicyDriverS3 {
		old, oldErr := s.decryptConfig(policy)
		if oldErr != nil && strings.TrimSpace(in.SecretKey) == "" {
			return PolicyView{}, ErrSecretUndecryptable
		}
		if strings.TrimSpace(in.SecretKey) == "" {
			in.SecretKey = old.SecretKey
		}
		var err error
		if sealed, err = s.sealInput(in); err != nil {
			return PolicyView{}, err
		}
	} else {
		sealed = "{}"
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if in.IsDefault && !policy.IsDefault {
			if err := tx.Model(&model.StoragePolicy{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Model(&policy).Updates(map[string]any{
			"name": strings.TrimSpace(in.Name), "driver": driver, "config": sealed,
			"url_mode": in.URLMode, "naming": in.Naming, "is_default": in.IsDefault, "status": in.Status,
		}).Error
	})
	if err != nil {
		return PolicyView{}, err
	}
	if err := s.db.First(&policy, id).Error; err != nil {
		return PolicyView{}, err
	}
	return s.view(policy)
}

// Delete 内置策略与有图引用的策略不可删。
func (s *PolicyService) Delete(id uint64) error {
	if id == model.PolicyLocalDefault {
		return errors.New("内置本地存储策略不可删除")
	}
	var count int64
	if err := s.db.Model(&model.Image{}).Where("policy_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrPolicyInUse
	}
	return s.db.Delete(&model.StoragePolicy{}, id).Error
}

// DefaultPolicyID 返回默认策略 ID（无则内置本地策略）。
func (s *PolicyService) DefaultPolicyID() (uint64, error) {
	var policy model.StoragePolicy
	if err := s.db.Where("is_default = ? AND status = ?", true, model.StatusActive).First(&policy).Error; err == nil {
		return policy.ID, nil
	}
	return model.PolicyLocalDefault, nil
}

// Driver 构造策略驱动。status=false 的策略不可用。
func (s *PolicyService) Driver(id uint64) (storage.Driver, model.StoragePolicy, error) {
	var policy model.StoragePolicy
	if err := s.db.First(&policy, id).Error; err != nil {
		return nil, policy, ErrPolicyNotFound
	}
	if policy.Status != model.StatusActive {
		return nil, policy, errors.New("存储策略已停用")
	}
	switch policy.Driver {
	case model.PolicyDriverLocal:
		return storage.NewLocal(localUploadsRoot(s.dataDir), s.base), policy, nil
	default:
		payload, err := s.decryptConfig(policy)
		if err != nil {
			return nil, policy, ErrSecretUndecryptable
		}
		driver, err := storage.BuildS3Driver(payload.S3Config, s.base)
		if err != nil {
			return nil, policy, err
		}
		return driver, policy, nil
	}
}

// Test 连通性检测（支持未保存的草稿配置）。
func (s *PolicyService) Test(ctx context.Context, in PolicyInput) error {
	if in.Driver == model.PolicyDriverS3 {
		payload := storage.S3Config{
			Endpoint: strings.TrimSpace(in.Endpoint), Region: strings.TrimSpace(in.Region),
			Bucket: strings.TrimSpace(in.Bucket), Prefix: strings.TrimSpace(in.Prefix),
			CDN: strings.TrimSpace(in.CDN), AccessKey: strings.TrimSpace(in.AccessKey),
			SecretKey: in.SecretKey, PathStyle: in.PathStyle,
		}
		// 未填 Secret 时用已存值（表单只回显 secret_configured 场景）
		if strings.TrimSpace(payload.SecretKey) == "" && in.ID != 0 {
			var policy model.StoragePolicy
			if err := s.db.First(&policy, in.ID).Error; err == nil {
				if old, err := s.decryptConfig(policy); err == nil {
					payload.SecretKey = old.SecretKey
				}
			}
		}
		driver, err := storage.BuildS3Driver(payload, s.base)
		if err != nil {
			return err
		}
		return driver.Check(ctx)
	}
	return nil
}

// EnsureSeedLocalPolicy 保证内置本地策略存在（种子缺失兜底）。
func (s *PolicyService) EnsureSeedLocalPolicy() error {
	var count int64
	if err := s.db.Model(&model.StoragePolicy{}).Where("id = ?", model.PolicyLocalDefault).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.StoragePolicy{
		ID: model.PolicyLocalDefault, Name: "本地存储", Driver: model.PolicyDriverLocal,
		Config: "{}", URLMode: model.PolicyURLDirect, Naming: model.NamingDate, IsDefault: true, Status: model.StatusActive,
	}).Error
}

// localUploadsRoot 本地驱动的文件根目录。
func localUploadsRoot(dataDir string) string {
	return dataDir + "/uploads"
}
