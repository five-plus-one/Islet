package service

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"islet/backend/internal/model"
)

var ErrSettingValue = errors.New("设置值过长")

// SettingsService system_configs 读写与进程内缓存（写后失效，读时懒加载 30s）。
type SettingsService struct {
	db *gorm.DB

	mu       sync.RWMutex
	cache    map[string]string
	loaded   bool
	loadedAt time.Time
}

func NewSettingsService(db *gorm.DB) *SettingsService {
	return &SettingsService{db: db}
}

// SettingDefaults 键的默认值（也是允许写入的白名单）。
func SettingDefaults() map[string]string {
	return map[string]string{
		SettingSiteTitle:      "Islet 图床",
		SettingPublicBaseURL:  "",
		SettingAutoApprove:    "false",
		SettingRetentionDays:  "30",
		SettingUploadMaxBytes: "104857600",
		SettingBrandName:      "Islet",
		SettingBrandLogo:      "",
		SettingBrandOIDCLabel: "OIDC 登录",
		SettingBrandOIDCLogo:  "",
		// 认证相关：空值表示回落到配置文件/环境变量
		SettingAuthOIDCIssuer:   "",
		SettingAuthOIDCClientID: "",
		SettingAuthOIDCAudience: "",
		SettingAuthOIDCScopes:   "",
		SettingAuthLocalEnabled: "true",
	}
}

// EffectiveAuth 生效的认证配置：数据库（系统设置）优先，配置文件/环境变量兜底。
type EffectiveAuth struct {
	Issuer       string
	ClientID     string
	Audience     string
	Scopes       string
	LocalEnabled bool
}

// ResolveAuth 计算生效认证配置。settings 为 nil 时直接用文件配置。
func ResolveAuth(settings *SettingsService, fallback EffectiveAuth) EffectiveAuth {
	result := fallback
	if result.Scopes == "" {
		result.Scopes = "openid profile email"
	}
	result.LocalEnabled = true
	if settings != nil {
		if v := strings.TrimSpace(settings.Get(SettingAuthOIDCIssuer)); v != "" {
			result.Issuer = v
		}
		if v := strings.TrimSpace(settings.Get(SettingAuthOIDCClientID)); v != "" {
			result.ClientID = v
		}
		if v := strings.TrimSpace(settings.Get(SettingAuthOIDCAudience)); v != "" {
			result.Audience = v
		}
		if v := strings.TrimSpace(settings.Get(SettingAuthOIDCScopes)); v != "" {
			result.Scopes = v
		}
		if strings.TrimSpace(settings.Get(SettingAuthLocalEnabled)) == "false" {
			result.LocalEnabled = false
		}
	}
	return result
}

func (s *SettingsService) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded && time.Since(s.loadedAt) < 30*time.Second {
		return nil
	}
	var rows []model.SystemConfig
	if err := s.db.Find(&rows).Error; err != nil {
		return err
	}
	cache := SettingDefaults()
	for _, row := range rows {
		cache[row.KeyName] = row.Value
	}
	s.cache = cache
	s.loaded = true
	s.loadedAt = time.Now()
	return nil
}

func (s *SettingsService) All() (map[string]string, error) {
	if err := s.load(); err != nil {
		return SettingDefaults(), err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string, len(s.cache))
	for k, v := range s.cache {
		result[k] = v
	}
	return result, nil
}

func (s *SettingsService) Get(key string) string {
	if err := s.load(); err != nil {
		return SettingDefaults()[key]
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[key]
}

func (s *SettingsService) GetBool(key string, fallback bool) bool {
	switch strings.TrimSpace(s.Get(key)) {
	case "true":
		return true
	case "false":
		return false
	}
	return fallback
}

func (s *SettingsService) GetInt64(key string, fallback int64) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s.Get(key)), 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

// Put 白名单内的键批量 upsert，随后失效缓存。
func (s *SettingsService) Put(values map[string]string) error {
	for key, value := range values {
		value = strings.TrimSpace(value)
		switch key {
		case SettingUploadMaxBytes:
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 1<<20 || n > 100<<20 {
				return errors.New("单文件上限须为 1–100 MB")
			}
		case SettingRetentionDays:
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || n > 3650 {
				return errors.New("保留时间须为 0–3650 天")
			}
		case SettingAuthLocalEnabled, SettingAutoApprove:
			if value != "true" && value != "false" {
				return errors.New("开关设置不合法")
			}
		case SettingPublicBaseURL, SettingAuthOIDCIssuer, SettingBrandLogo, SettingBrandOIDCLogo:
			if value != "" {
				u, err := url.Parse(value)
				if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" || (key == SettingPublicBaseURL && u.RawQuery != "") {
					return errors.New("请填写完整的 http 或 https 地址")
				}
			}
		}
	}
	allowed := SettingDefaults()
	rows := make([]model.SystemConfig, 0, len(values))
	for key, value := range values {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if len(value) > 2048 {
			return ErrSettingValue
		}
		rows = append(rows, model.SystemConfig{KeyName: key, Value: strings.TrimSpace(value)})
	}
	if len(rows) == 0 {
		return nil
	}
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&rows).Error
	s.mu.Lock()
	s.loaded = false
	s.mu.Unlock()
	return err
}

// Invalidate 供外部直接改表后清缓存（如初始化种子写入）。
func (s *SettingsService) Invalidate() {
	s.mu.Lock()
	s.loaded = false
	s.mu.Unlock()
}
