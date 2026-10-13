package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	"islet/backend/internal/model"
)

var ErrTokenNotFound = errors.New("token not found")

// APITokenService 供 PicGo / 脚本上传用的 Bearer Token（库存 SHA-256 哈希）。
type APITokenService struct {
	db *gorm.DB
}

func NewAPITokenService(db *gorm.DB) *APITokenService { return &APITokenService{db: db} }

type TokenView struct {
	ID         uint64     `json:"id"`
	Name       string     `json:"name"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
}

func tokenView(t model.APIToken) TokenView {
	return TokenView{
		ID: t.ID, Name: t.Name, ExpiresAt: t.ExpiresAt,
		LastUsedAt: t.LastUsedAt, Status: t.Status, CreatedAt: t.CreatedAt,
	}
}

// Create 返回仅此一次的明文 token（isl_ + 32 hex）。
func (s *APITokenService) Create(userID uint64, name string, expiresAt *time.Time) (string, TokenView, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", TokenView{}, err
	}
	plaintext := "isl_" + hex.EncodeToString(raw)
	token := model.APIToken{
		UserID: userID, Name: name, TokenHash: hashToken(plaintext),
		ExpiresAt: expiresAt, Status: model.StatusActive,
	}
	if err := s.db.Create(&token).Error; err != nil {
		return "", TokenView{}, err
	}
	return plaintext, tokenView(token), nil
}

// Resolve 校验并解析 token，命中后异步更新 last_used_at。
func (s *APITokenService) Resolve(plaintext string) (model.APIToken, model.User, error) {
	var token model.APIToken
	if err := s.db.Where("token_hash = ? AND status = ?", hashToken(plaintext), model.StatusActive).First(&token).Error; err != nil {
		return token, model.User{}, ErrTokenNotFound
	}
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return token, model.User{}, ErrTokenNotFound
	}
	var user model.User
	if err := s.db.First(&user, token.UserID).Error; err != nil {
		return token, user, ErrTokenNotFound
	}
	if user.Status != model.StatusActive {
		return token, user, ErrAccountUnavailable
	}
	now := time.Now()
	_ = s.db.Model(&token).Update("last_used_at", now).Error
	return token, user, nil
}

func (s *APITokenService) List(userID uint64) ([]TokenView, error) {
	var tokens []model.APIToken
	if err := s.db.Where("user_id = ?", userID).Order("id DESC").Find(&tokens).Error; err != nil {
		return nil, err
	}
	views := make([]TokenView, 0, len(tokens))
	for _, t := range tokens {
		views = append(views, tokenView(t))
	}
	return views, nil
}

func (s *APITokenService) Revoke(userID, id uint64) error {
	result := s.db.Model(&model.APIToken{}).Where("id = ? AND user_id = ?", id, userID).Update("status", model.StatusDisabled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (s *APITokenService) RevokeUserTokens(userID uint64) error {
	return s.db.Model(&model.APIToken{}).Where("user_id = ?", userID).Update("status", "disabled").Error
}
