package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"islet/backend/internal/config"
	"islet/backend/internal/model"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountUnavailable = errors.New("account unavailable")
)

const localSessionTTL = 12 * time.Hour

const (
	SettingAutoApprove      = "users.auto_approve"
	SettingRetentionDays    = "recycle.retention_days"
	SettingPublicBaseURL    = "site.public_base_url"
	SettingSiteTitle        = "site.title"
	SettingUploadMaxBytes   = "upload.max_bytes"
	SettingBrandName        = "brand.name"
	SettingBrandLogo        = "brand.logo_url"
	SettingBrandOIDCLabel   = "brand.oidc_label"
	SettingBrandOIDCLogo    = "brand.oidc_logo_url"
	SettingAuthOIDCIssuer   = "auth.oidc_issuer"
	SettingAuthOIDCClientID = "auth.oidc_client_id"
	SettingAuthOIDCAudience = "auth.oidc_audience"
	SettingAuthOIDCScopes   = "auth.oidc_scopes"
	SettingAuthLocalEnabled = "auth.local_enabled"
)

type AuthService struct {
	db *gorm.DB
}

func NewAuthService(db *gorm.DB) *AuthService { return &AuthService{db: db} }

func (s *AuthService) Installed() (bool, error) {
	var state model.InstallationState
	err := s.db.First(&state, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return state.Installed && state.OwnerUserID != nil, err
}

func HashPassword(password string) (string, error) {
	if len(password) < 10 || len(password) > 72 {
		return "", errors.New("密码长度须为 10–72 字节")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// CreateBootstrapOwner 初始化收尾：事务内认领首个 super_admin（防并发重复认领）。
func (s *AuthService) CreateBootstrapOwner(cfg config.Config) (model.User, error) {
	var owner model.User
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var state model.InstallationState
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, 1).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			state = model.InstallationState{ID: 1, SchemaVersion: 1}
			if err := tx.Create(&state).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if state.Installed && state.OwnerUserID != nil {
			return tx.First(&owner, *state.OwnerUserID).Error
		}
		if !cfg.BootstrapVerified {
			return errors.New("administrator identity has not been verified")
		}

		switch cfg.AuthMode {
		case model.AuthSourceLocal:
			err = tx.Where("auth_source = ? AND username = ?", model.AuthSourceLocal, cfg.BootstrapUsername).First(&owner).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				owner = model.User{
					AuthSource: model.AuthSourceLocal, Username: cfg.BootstrapUsername,
					PasswordHash: &cfg.BootstrapPassword, Role: model.UserRoleSuperAdmin,
					GroupID: model.GroupAdministrators, Status: model.StatusActive,
				}
				if err := tx.Create(&owner).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		default:
			err = tx.Where("sso_id = ?", cfg.BootstrapSubject).First(&owner).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				owner = model.User{
					AuthSource: model.AuthSourceOIDC, SSOID: &cfg.BootstrapSubject, Username: cfg.BootstrapUsername,
					Email: optionalString(cfg.BootstrapEmail), Role: model.UserRoleSuperAdmin,
					GroupID: model.GroupAdministrators, Status: model.StatusActive,
				}
				if err := tx.Create(&owner).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}

		if err := tx.Model(&owner).Updates(map[string]any{
			"role": model.UserRoleSuperAdmin, "status": model.StatusActive, "group_id": model.GroupAdministrators,
		}).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		state.Installed = true
		state.OwnerUserID = &owner.ID
		state.SchemaVersion = 1
		state.InstalledAt = &now
		return tx.Save(&state).Error
	})
	return owner, err
}

// ProvisionOIDC 首登建号：默认分组；默认待审核（system_configs users.auto_approve 可放开）。
func (s *AuthService) ProvisionOIDC(identity Identity) (model.User, error) {
	var user model.User
	err := s.db.Where("sso_id = ?", identity.Subject).First(&user).Error
	if err == nil {
		updates := map[string]any{}
		if identity.Username != "" && identity.Username != user.Username {
			updates["username"] = identity.Username
			user.Username = identity.Username
		}
		if identity.Email != "" && (user.Email == nil || *user.Email != identity.Email) {
			updates["email"] = identity.Email
		}
		if len(updates) > 0 {
			if err := s.db.Model(&user).Updates(updates).Error; err != nil {
				return user, err
			}
		}
		return user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return user, err
	}
	status := model.StatusPending
	if boolSetting(s.db, SettingAutoApprove, false) {
		status = model.StatusActive
	}
	subject := identity.Subject
	username := firstNonEmpty(identity.Username, identity.Email, "oidc-"+subject)
	if len(username) > 64 {
		username = username[:64]
	}
	user = model.User{
		AuthSource: model.AuthSourceOIDC, SSOID: &subject, Username: username,
		Email: optionalString(identity.Email), Role: model.UserRoleUser,
		GroupID: model.GroupMembers, Status: status,
	}
	err = s.db.Create(&user).Error
	if err != nil {
		// 并发首登兜底：唯一键冲突后再查一次
		if lookupErr := s.db.Where("sso_id = ?", subject).First(&user).Error; lookupErr == nil {
			return user, nil
		}
	}
	return user, err
}

func (s *AuthService) LoginLocal(username, password string) (model.User, string, error) {
	var user model.User
	if err := s.db.Where("auth_source = ? AND username = ?", model.AuthSourceLocal, strings.TrimSpace(username)).First(&user).Error; err != nil {
		return user, "", ErrInvalidCredentials
	}
	if user.PasswordHash == nil || bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)) != nil {
		return user, "", ErrInvalidCredentials
	}
	if user.Status != model.StatusActive {
		return user, "", ErrAccountUnavailable
	}
	token, hash, err := newSessionToken()
	if err != nil {
		return user, "", err
	}
	now := time.Now().UTC()
	session := model.AuthSession{UserID: user.ID, TokenHash: hash, ExpiresAt: now.Add(localSessionTTL), LastSeenAt: &now}
	if err := s.db.Create(&session).Error; err != nil {
		return user, "", err
	}
	_ = s.db.Model(&user).Update("last_login_at", now).Error
	return user, token, nil
}

func (s *AuthService) VerifyLocalPassword(userID uint64, password string) error {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return ErrInvalidCredentials
	}
	if user.AuthSource != model.AuthSourceLocal || user.PasswordHash == nil {
		return ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)) != nil {
		return ErrInvalidCredentials
	}
	return nil
}

func (s *AuthService) ResolveSession(token string) (model.User, error) {
	var user model.User
	hash := hashToken(token)
	err := s.db.Table("users").
		Joins("JOIN auth_sessions ON auth_sessions.user_id = users.id").
		Where("auth_sessions.token_hash = ? AND auth_sessions.expires_at > ? AND users.status = ?", hash, time.Now().UTC(), model.StatusActive).
		Where("users.deleted_at IS NULL").
		First(&user).Error
	if err != nil {
		return user, ErrInvalidCredentials
	}
	return user, nil
}

func (s *AuthService) RevokeSession(token string) error {
	if token == "" {
		return nil
	}
	return s.db.Where("token_hash = ?", hashToken(token)).Delete(&model.AuthSession{}).Error
}

func (s *AuthService) RevokeUserSessions(userID uint64) error {
	return s.db.Where("user_id = ?", userID).Delete(&model.AuthSession{}).Error
}

func newSessionToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func boolSetting(db *gorm.DB, key string, fallback bool) bool {
	var row model.SystemConfig
	if err := db.Where("key_name = ?", key).First(&row).Error; err != nil {
		return fallback
	}
	return row.Value == "true"
}
