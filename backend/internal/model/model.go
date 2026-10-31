package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

const (
	UserRoleSuperAdmin = "super_admin"
	UserRoleAdmin      = "admin"
	UserRoleUser       = "user"

	AuthSourceOIDC  = "oidc"
	AuthSourceLocal = "local"

	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusPending  = "pending"

	PolicyDriverLocal = "local"
	PolicyDriverS3    = "s3"

	PolicyURLDirect = "direct"
	PolicyURLProxy  = "proxy"

	NamingKeep   = "keep"
	NamingRandom = "random"
	NamingDate   = "date"

	// 内置分组 ID
	GroupAdministrators uint64 = 1
	GroupMembers        uint64 = 2
	// 内置存储策略 ID
	PolicyLocalDefault uint64 = 1
)

type User struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement"`
	AuthSource   string         `gorm:"type:enum('oidc','local');not null;default:oidc"`
	SSOID        *string        `gorm:"column:sso_id;size:128;uniqueIndex:uk_sso_id"`
	Username     string         `gorm:"size:64;not null;uniqueIndex:uk_username"`
	Email        *string        `gorm:"size:128;uniqueIndex:uk_email"`
	PasswordHash *string        `gorm:"size:255"`
	Role         string         `gorm:"type:enum('super_admin','admin','user');not null;default:user"`
	GroupID      uint64         `gorm:"not null;default:2;index:idx_user_group"`
	Status       string         `gorm:"type:enum('pending','active','disabled');not null;default:pending;index:idx_user_status"`
	LastLoginAt  *time.Time     `gorm:"index:idx_user_last_login"`
	CreatedAt    time.Time      `gorm:"not null"`
	UpdatedAt    time.Time      `gorm:"not null"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

type AuthSession struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	UserID     uint64    `gorm:"not null;index:idx_session_user"`
	TokenHash  string    `gorm:"size:64;not null;uniqueIndex:uk_token_hash"`
	ExpiresAt  time.Time `gorm:"not null;index:idx_session_expire"`
	LastSeenAt *time.Time
	CreatedAt  time.Time `gorm:"not null"`
}

type InstallationState struct {
	ID            uint8 `gorm:"primaryKey"`
	Installed     bool  `gorm:"not null;default:false"`
	OwnerUserID   *uint64
	SchemaVersion uint `gorm:"not null;default:1"`
	InstalledAt   *time.Time
	UpdatedAt     time.Time `gorm:"not null"`
}

type AuditLog struct {
	ID         uint64          `gorm:"primaryKey;autoIncrement"`
	UserID     *uint64         `gorm:"index:idx_audit_user"`
	Action     string          `gorm:"size:64;not null;index:idx_audit_action"`
	TargetType *string         `gorm:"size:64"`
	TargetID   *string         `gorm:"size:128"`
	Detail     json.RawMessage `gorm:"type:json"`
	IP         *string         `gorm:"size:45"`
	CreatedAt  time.Time       `gorm:"not null;index:idx_audit_created"`
}

// UserGroup 用户分组：管「能用多少、能传什么、存到哪里」，与管理权限（role）无关。
type UserGroup struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	Name           string    `gorm:"size:64;not null;uniqueIndex:uk_group_name"`
	Description    string    `gorm:"size:255"`
	QuotaBytes     uint64    `gorm:"not null;default:0"`        // 0=不限
	MaxFileBytes   uint64    `gorm:"not null;default:0"`        // 0=不限
	AllowedMimes   []string  `gorm:"serializer:json;type:json"` // 空=全部 image/*
	PolicyIDs      []uint64  `gorm:"serializer:json;type:json"` // 空=全部策略
	UploadsPerHour int       `gorm:"not null;default:0"`        // 0=不限
	BuiltIn        bool      `gorm:"not null;default:false"`
	Status         string    `gorm:"type:enum('active','disabled');not null;default:active"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (UserGroup) TableName() string { return "user_groups" }

type StoragePolicy struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	Name      string         `gorm:"size:64;not null;uniqueIndex:uk_policy_name"`
	Driver    string         `gorm:"type:enum('local','s3');not null"`
	Config    string         `gorm:"type:text;not null"` // AES-GCM 密文
	URLMode   string         `gorm:"type:enum('direct','proxy');not null;default:direct"`
	Naming    string         `gorm:"type:enum('keep','random','date');not null;default:random"`
	IsDefault bool           `gorm:"not null;default:false"`
	Status    string         `gorm:"type:enum('active','disabled');not null;default:active"`
	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (StoragePolicy) TableName() string { return "storage_policies" }

type Image struct {
	ID           uint64   `gorm:"primaryKey;autoIncrement"`
	UserID       uint64   `gorm:"not null;index:idx_image_user"`
	PolicyID     uint64   `gorm:"not null"`
	AlbumID      *uint64  `gorm:"index:idx_image_album"`
	StorageKey   string   `gorm:"size:512;not null;uniqueIndex:uk_storage_key"`
	URL          string   `gorm:"size:1024;not null"`
	ThumbKey     *string  `gorm:"size:512;index:idx_image_thumb"`
	ThumbURL     *string  `gorm:"size:1024"`
	OriginalName string   `gorm:"size:255;not null"`
	Mime         string   `gorm:"size:128;not null;index:idx_image_mime"`
	Size         uint64   `gorm:"not null"`
	Width        int      `gorm:"not null;default:0"`
	Height       int      `gorm:"not null;default:0"`
	SHA256       string   `gorm:"column:sha256;size:64;not null;index:idx_image_sha"`
	Tags         []string `gorm:"serializer:json;type:json"`
	Blocked      bool     `gorm:"not null;default:false;index:idx_image_blocked"`
	BlockReason  string   `gorm:"size:2000;not null;default:''"`
	ModeratedBy  *uint64
	ModeratedAt  *time.Time
	CreatedAt    time.Time      `gorm:"not null;index:idx_image_created"`
	UpdatedAt    time.Time      `gorm:"not null"`
	DeletedAt    gorm.DeletedAt `gorm:"index:idx_image_deleted"`
}

type Album struct {
	ID          uint64         `gorm:"primaryKey;autoIncrement"`
	UserID      uint64         `gorm:"not null;index:idx_album_user"`
	Name        string         `gorm:"size:64;not null"`
	Description string         `gorm:"size:255"`
	CreatedAt   time.Time      `gorm:"not null"`
	UpdatedAt   time.Time      `gorm:"not null"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// CDNCacheJob survives image deletion and process restarts.
type CDNCacheJob struct {
	ID            uint64   `gorm:"primaryKey;autoIncrement"`
	ImageID       uint64   `gorm:"not null;index"`
	PolicyID      uint64   `gorm:"not null"`
	URLs          []string `gorm:"serializer:json;type:json;not null"`
	TaskIDs       string   `gorm:"size:2048;not null;default:''"`
	SubmittedAt   *time.Time
	Status        string    `gorm:"size:16;not null;default:pending;index:idx_cdn_due,priority:1"`
	Attempts      int       `gorm:"not null;default:0"`
	NextAttemptAt time.Time `gorm:"not null;index:idx_cdn_due,priority:2"`
	LastError     string    `gorm:"size:128;not null;default:''"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (CDNCacheJob) TableName() string { return "cdn_cache_jobs" }

type APIToken struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	UserID     uint64 `gorm:"not null;index:idx_token_user"`
	Name       string `gorm:"size:64;not null"`
	TokenHash  string `gorm:"size:64;not null;uniqueIndex:uk_api_token_hash"`
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	Status     string    `gorm:"type:enum('active','disabled');not null;default:active"`
	CreatedAt  time.Time `gorm:"not null"`
}

func (APIToken) TableName() string { return "api_tokens" }

type SystemConfig struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	KeyName     string    `gorm:"column:key_name;size:128;not null;uniqueIndex:uk_key_name"`
	Value       string    `gorm:"type:text;not null"`
	Description *string   `gorm:"size:255"`
	UpdatedAt   time.Time `gorm:"not null"`
}

// AllModels AutoMigrate 全集。
func AllModels() []any {
	return []any{
		&User{}, &AuthSession{}, &InstallationState{}, &AuditLog{},
		&UserGroup{}, &StoragePolicy{}, &Image{}, &Album{}, &APIToken{}, &SystemConfig{}, &CDNCacheJob{},
	}
}
