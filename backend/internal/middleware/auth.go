package middleware

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"islet/backend/internal/config"
	"islet/backend/internal/model"
	"islet/backend/internal/response"
	"islet/backend/internal/service"
)

const (
	ContextUserKey     = "islet_user"
	LocalSessionCookie = "islet_session"
)

// AuthUser 请求上下文中的用户身份。
type AuthUser struct {
	ID         uint64 `json:"id"`
	Subject    string `json:"subject"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	AuthSource string `json:"auth_source"`
	GroupID    uint64 `json:"group_id"`
	ViaToken   bool   `json:"via_token"`
}

// Authenticator 聚合三种凭证解析：会话 Cookie、OIDC Bearer、API Token。
type Authenticator struct {
	cfg      func() config.Config
	logger   *slog.Logger
	db       *gorm.DB
	oidc     *service.OIDCService
	tokens   *service.APITokenService
	authSvc  *service.AuthService
	settings *service.SettingsService
}

func NewAuthenticator(cfg func() config.Config, logger *slog.Logger, db *gorm.DB, oidc *service.OIDCService, tokens *service.APITokenService, settings *service.SettingsService) *Authenticator {
	return &Authenticator{cfg: cfg, logger: logger, db: db, oidc: oidc, tokens: tokens, authSvc: service.NewAuthService(db), settings: settings}
}

func (a *Authenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if user, ok := a.fromSession(c); ok {
			c.Set(ContextUserKey, user)
			c.Next()
			return
		}
		if user, ok := a.fromBearer(c); ok {
			c.Set(ContextUserKey, user)
			c.Next()
			return
		}
		response.Unauthorized(c, "unauthorized")
		c.Abort()
	}
}

func (a *Authenticator) fromSession(c *gin.Context) (AuthUser, bool) {
	token, err := c.Cookie(LocalSessionCookie)
	if err != nil || token == "" {
		return AuthUser{}, false
	}
	user, err := a.authSvc.ResolveSession(token)
	if err != nil {
		return AuthUser{}, false
	}
	return FromModel(user), true
}

func (a *Authenticator) fromBearer(c *gin.Context) (AuthUser, bool) {
	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		return AuthUser{}, false
	}
	cfg := a.cfg()
	// 生效认证配置：system_configs（系统设置页可改）优先，配置文件兜底
	fallback := service.EffectiveAuth{
		Issuer: cfg.OIDCIssuer, ClientID: cfg.OIDCClientID,
		Audience: cfg.OIDCAudience, Scopes: cfg.OIDCScopes,
	}
	auth := service.ResolveAuth(a.settings, fallback)

	// 先按 OIDC access token 验（标准 Discovery + JWKS）
	if auth.Issuer != "" && auth.ClientID != "" {
		audience := auth.Audience
		if audience == "" {
			audience = auth.ClientID
		}
		identity, err := a.oidc.Verify(c.Request.Context(), token, service.VerifyOptions{
			Issuer: auth.Issuer, Audience: audience,
		})
		if err == nil {
			if raw := c.GetHeader("X-OIDC-ID-Token"); raw != "" {
				if profile, perr := a.oidc.Verify(c.Request.Context(), raw, service.VerifyOptions{
					Issuer: auth.Issuer, Audience: auth.ClientID,
				}); perr == nil && profile.Subject == identity.Subject {
					identity.Username = firstNonEmpty(profile.Username, identity.Username)
					identity.Email = firstNonEmpty(profile.Email, identity.Email)
				}
			}
			user, err := a.authSvc.ProvisionOIDC(identity)
			if err != nil {
				a.logger.Warn("provision oidc user failed", "error", err)
				return AuthUser{}, false
			}
			if user.Status != model.StatusActive {
				return AuthUser{}, false
			}
			return FromModel(user), true
		}
	}

	// 再按 API Token 解
	if a.tokens != nil && strings.HasPrefix(token, "isl_") {
		_, user, err := a.tokens.Resolve(token)
		if err == nil {
			result := FromModel(user)
			result.ViaToken = true
			return result, true
		}
	}
	return AuthUser{}, false
}

func FromModel(user model.User) AuthUser {
	result := AuthUser{
		ID: user.ID, Username: user.Username, Role: user.Role, Status: user.Status,
		AuthSource: user.AuthSource, GroupID: user.GroupID,
	}
	if user.SSOID != nil {
		result.Subject = *user.SSOID
	}
	if user.Email != nil {
		result.Email = *user.Email
	}
	return result
}

// CurrentUser 取当前请求用户；未认证时第二个返回值为 false。
func CurrentUser(c *gin.Context) (AuthUser, bool) {
	value, exists := c.Get(ContextUserKey)
	user, ok := value.(AuthUser)
	return user, exists && ok
}

// RequireRole 允许的角色（super_admin 恒通过）。
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := CurrentUser(c)
		if !ok {
			response.Unauthorized(c, "unauthorized")
			c.Abort()
			return
		}
		if user.Role == model.UserRoleSuperAdmin {
			c.Next()
			return
		}
		for _, role := range roles {
			if user.Role == role {
				c.Next()
				return
			}
		}
		response.Forbidden(c, "forbidden")
		c.Abort()
	}
}

func RequireSuperAdmin() gin.HandlerFunc {
	return RequireRole(model.UserRoleSuperAdmin)
}

// RequireInteractive keeps upload credentials out of account and administration APIs.
func RequireInteractive() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := CurrentUser(c)
		if !ok || user.ViaToken {
			response.Forbidden(c, "上传密钥仅可用于上传图片")
			c.Abort()
			return
		}
		c.Next()
	}
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
