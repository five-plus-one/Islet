package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"islet/backend/internal/config"
	"islet/backend/internal/database"
	"islet/backend/internal/middleware"
	"islet/backend/internal/model"
	"islet/backend/internal/response"
	"islet/backend/internal/service"
)

const errSetupLocked = "setup_locked"

// setupLocked 初始化完成后锁定全部写接口（以配置文件安装状态为准，不依赖 DB）。
func setupLocked(c *gin.Context) bool {
	if config.Load().Complete() {
		response.Error(c, http.StatusForbidden, 4403, errSetupLocked)
		return true
	}
	return false
}

// setupDeps 从引擎约定读取共享 deps（RegisterSetupRoutes 注入）。
func setupDeps(c *gin.Context) *Deps {
	value, ok := c.Get("islet_deps")
	if !ok {
		return nil
	}
	deps, _ := value.(*Deps)
	return deps
}

func registerSetupRoutes(setup *gin.RouterGroup, deps *Deps) {
	setup.Use(func(c *gin.Context) {
		c.Set("islet_deps", deps)
		c.Next()
	})
	setup.GET("/status", handleSetupStatus)
	setup.POST("/test/database", handleSetupTestDatabase)
	setup.POST("/test/oidc", handleSetupTestOIDC)
	setup.POST("/verify/admin", handleSetupVerifyAdmin)
	setup.POST("/complete", handleSetupComplete)
}

type setupStatusPayload struct {
	Status     string            `json:"status"`
	AuthMode   string            `json:"auth_mode"`
	Verified   bool              `json:"admin_verified"`
	Defaults   map[string]string `json:"defaults"`
	AuthConfig authConfigPayload `json:"auth_config"`
}

func handleSetupStatus(c *gin.Context) {
	cfg := config.Load()
	status := "setup_required"
	if cfg.Complete() {
		status = "ready"
	}
	verified := cfg.BootstrapVerified
	defaults := map[string]string{
		"mysql_host": cfg.MySQLHost, "mysql_port": cfg.MySQLPort,
		"mysql_database": cfg.MySQLDatabase, "mysql_user": cfg.MySQLUser,
		"redis_host": cfg.RedisHost, "redis_port": cfg.RedisPort,
	}
	payload := setupStatusPayload{
		Status: status, AuthMode: cfg.AuthMode, Verified: verified, Defaults: defaults,
		AuthConfig: buildAuthConfig(cfg, c.Request, nil),
	}
	response.OK(c, payload)
}

type databaseInput struct {
	MySQLHost     string `json:"mysql_host"`
	MySQLPort     string `json:"mysql_port"`
	MySQLDatabase string `json:"mysql_database"`
	MySQLUser     string `json:"mysql_user"`
	MySQLPassword string `json:"mysql_password"`
	RedisHost     string `json:"redis_host"`
	RedisPort     string `json:"redis_port"`
	RedisPassword string `json:"redis_password"`
	RedisDB       int    `json:"redis_db"`
}

// handleSetupTestDatabase 测 MySQL / Redis 连通。
func handleSetupTestDatabase(c *gin.Context) {
	if setupLocked(c) {
		return
	}
	var in databaseInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数不完整")
		return
	}
	if in.MySQLPort == "" {
		in.MySQLPort = "3306"
	}
	if in.MySQLDatabase == "" {
		in.MySQLDatabase = "islet"
	}
	result := map[string]bool{"mysql": false, "redis": false}
	mysqlErr := ""
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		in.MySQLUser, in.MySQLPassword, in.MySQLHost, in.MySQLPort, in.MySQLDatabase)
	if db, err := database.ConnectMySQL(dsn); err != nil {
		mysqlErr = err.Error()
	} else {
		result["mysql"] = true
		if pool, err := db.DB(); err == nil {
			_ = pool.Close()
		}
	}
	redisErr := ""
	if in.RedisHost != "" {
		if in.RedisPort == "" {
			in.RedisPort = "6379"
		}
		addr := in.RedisHost + ":" + in.RedisPort
		if client, err := database.ConnectRedis(addr, in.RedisPassword, in.RedisDB); err != nil {
			redisErr = err.Error()
		} else {
			result["redis"] = true
			_ = client.Close()
		}
	} else {
		result["redis"] = true // Redis 未填视为可选项跳过（上传限流降级）
	}
	payload := gin.H{"mysql": result["mysql"], "redis": result["redis"]}
	if mysqlErr != "" {
		payload["mysql_error"] = mysqlErr
	}
	if redisErr != "" {
		payload["redis_error"] = redisErr
	}
	if !result["mysql"] {
		response.Error(c, http.StatusBadRequest, 4002, "MySQL 连接失败")
		return
	}
	response.OK(c, payload)
}

// handleSetupTestOIDC 标准 OIDC Discovery 检查，通过后把连接信息持久化到配置文件。
func handleSetupTestOIDC(c *gin.Context) {
	if setupLocked(c) {
		return
	}
	var in struct {
		Issuer   string `json:"issuer"`
		ClientID string `json:"client_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Issuer) == "" {
		response.BadRequest(c, "请填写 Issuer")
		return
	}
	oidc := service.NewOIDCService()
	meta, err := oidc.Discover(c.Request.Context(), in.Issuer)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// Discovery 通过即落盘，后续 verify/login 直接可用
	cfg := config.Load()
	if strings.TrimSpace(in.Issuer) != "" {
		cfg.OIDCIssuer = strings.TrimSpace(in.Issuer)
	}
	if strings.TrimSpace(in.ClientID) != "" {
		cfg.OIDCClientID = strings.TrimSpace(in.ClientID)
	}
	if err := config.SaveFile(cfg.ConfigFile, cfg); err != nil {
		response.ServerError(c, "写入配置失败："+err.Error())
		return
	}
	response.OK(c, gin.H{
		"issuer":                 meta.Issuer,
		"authorization_endpoint": meta.AuthorizationEndpoint,
		"token_endpoint":         meta.TokenEndpoint,
		"jwks_uri":               meta.JWKSURI,
		"end_session_endpoint":   meta.EndSessionEndpoint,
	})
}

// handleSetupVerifyAdmin 暂存管理员身份：OIDC token 验签或本地账号密码哈希。
func handleSetupVerifyAdmin(c *gin.Context) {
	if setupLocked(c) {
		return
	}
	var in struct {
		Mode     string `json:"mode"` // oidc | local
		Token    string `json:"token"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数不完整")
		return
	}
	cfg := config.Load()
	switch strings.TrimSpace(in.Mode) {
	case model.AuthSourceOIDC:
		if cfg.OIDCIssuer == "" || cfg.OIDCClientID == "" {
			response.BadRequest(c, "请先在向导中填写 Issuer 与 Client ID 并通过 Discovery 检查")
			return
		}
		audience := cfg.OIDCAudience
		if audience == "" {
			audience = cfg.OIDCClientID
		}
		identity, err := depsOIDC().Verify(c.Request.Context(), in.Token, service.VerifyOptions{
			Issuer: cfg.OIDCIssuer, Audience: audience,
		})
		if err != nil {
			response.Unauthorized(c, "管理员身份验证失败："+err.Error())
			return
		}
		username := identity.Username
		if username == "" {
			username = identity.Email
		}
		if username == "" {
			username = "admin"
		}
		cfg.AuthMode = model.AuthSourceOIDC
		cfg.BootstrapVerified = true
		cfg.BootstrapSubject = identity.Subject
		cfg.BootstrapUsername = username
		cfg.BootstrapEmail = identity.Email
		if err := config.SaveFile(cfg.ConfigFile, cfg); err != nil {
			response.ServerError(c, "写入配置失败："+err.Error())
			return
		}
		response.OK(c, gin.H{"mode": "oidc", "username": username, "subject": identity.Subject})
	case model.AuthSourceLocal:
		if strings.TrimSpace(in.Username) == "" {
			response.BadRequest(c, "用户名不能为空")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil || len(in.Password) < 10 {
			response.BadRequest(c, "密码至少需要 10 位")
			return
		}
		cfg.AuthMode = model.AuthSourceLocal
		cfg.BootstrapVerified = true
		cfg.BootstrapUsername = strings.TrimSpace(in.Username)
		cfg.BootstrapPassword = string(hash)
		if err := config.SaveFile(cfg.ConfigFile, cfg); err != nil {
			response.ServerError(c, "写入配置失败："+err.Error())
			return
		}
		response.OK(c, gin.H{"mode": "local", "username": cfg.BootstrapUsername})
	default:
		response.BadRequest(c, "mode 须为 oidc 或 local")
	}
}

func depsOIDC() *service.OIDCService { return service.NewOIDCService() }

// handleSetupComplete 建表、种子、认领 owner、锁定、热启用。
// 数据库连接信息在此步一并落盘（向导第 1 步只测试不保存，凭据不驻留服务端）。
func handleSetupComplete(c *gin.Context) {
	if setupLocked(c) {
		return
	}
	var in struct {
		Database      databaseInput `json:"database"`
		SiteTitle     string        `json:"site_title"`
		PublicBaseURL string        `json:"public_base_url"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数不完整")
		return
	}
	cfg := config.Load()
	// 合并向导提交的数据库配置
	if strings.TrimSpace(in.Database.MySQLHost) != "" {
		cfg.MySQLHost = strings.TrimSpace(in.Database.MySQLHost)
		cfg.MySQLPort = strings.TrimSpace(in.Database.MySQLPort)
		if cfg.MySQLPort == "" {
			cfg.MySQLPort = "3306"
		}
		cfg.MySQLDatabase = strings.TrimSpace(in.Database.MySQLDatabase)
		if cfg.MySQLDatabase == "" {
			cfg.MySQLDatabase = "islet"
		}
		cfg.MySQLUser = strings.TrimSpace(in.Database.MySQLUser)
		cfg.MySQLPassword = in.Database.MySQLPassword
	}
	if strings.TrimSpace(in.Database.RedisHost) != "" {
		cfg.RedisHost = strings.TrimSpace(in.Database.RedisHost)
		cfg.RedisPort = strings.TrimSpace(in.Database.RedisPort)
		if cfg.RedisPort == "" {
			cfg.RedisPort = "6379"
		}
		cfg.RedisPassword = in.Database.RedisPassword
	}
	if !cfg.DBReady() {
		response.BadRequest(c, "缺少数据库连接信息")
		return
	}
	if !cfg.AuthReady() || !cfg.BootstrapVerified {
		response.BadRequest(c, "请先完成管理员身份验证")
		return
	}
	db, err := database.ConnectMySQL(cfg.DSN())
	if err != nil {
		cfg.InstallationComplete = false
		_ = config.SaveFile(cfg.ConfigFile, cfg)
		response.BadRequest(c, "MySQL 连接失败："+err.Error())
		return
	}
	if pool, err := db.DB(); err == nil {
		defer pool.Close()
	}
	if err := database.Migrate(db); err != nil {
		response.ServerError(c, "数据库迁移失败："+err.Error())
		return
	}
	if err := database.Seed(db, strings.TrimRight(strings.TrimSpace(in.PublicBaseURL), "/")); err != nil {
		response.ServerError(c, "种子数据写入失败："+err.Error())
		return
	}

	owner, err := service.NewAuthService(db).CreateBootstrapOwner(cfg)
	if err != nil {
		response.ServerError(c, "管理员认领失败："+err.Error())
		return
	}

	// 站点设置
	settings := service.NewSettingsService(db)
	values := map[string]string{}
	if strings.TrimSpace(in.SiteTitle) != "" {
		values[service.SettingSiteTitle] = strings.TrimSpace(in.SiteTitle)
	}
	if strings.TrimSpace(in.PublicBaseURL) != "" {
		values[service.SettingPublicBaseURL] = strings.TrimRight(strings.TrimSpace(in.PublicBaseURL), "/")
	}
	if len(values) > 0 {
		if err := settings.Put(values); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}

	// 清理配置文件中的临时凭据
	cfg.BootstrapVerified = false
	cfg.BootstrapSubject = ""
	cfg.BootstrapUsername = ""
	cfg.BootstrapEmail = ""
	cfg.BootstrapPassword = ""
	cfg.InstallationComplete = true
	cfg.ResetPending = false
	if err := config.SaveFile(cfg.ConfigFile, cfg); err != nil {
		response.ServerError(c, "保存配置失败，请重试")
		return
	}
	if err := config.ClearResetMarker(cfg.ConfigFile); err != nil {
		response.ServerError(c, "更新初始化状态失败，请重试")
		return
	}

	// 热启用业务（失败不影响初始化结果，重启后自愈）
	if deps := setupDeps(c); deps != nil && deps.OnSetupComplete != nil {
		if err := deps.OnSetupComplete(); err != nil {
			deps.Logger.Warn("bootstrap after setup failed", "error", err)
		}
	}

	_ = db.Where("1 = 1").Delete(&model.AuthSession{}).Error
	response.OK(c, gin.H{"owner": owner.Username, "status": "installed"})
}

// ---- 认证配置 payload（登录页 / 向导页共用） ----

type authConfigPayload struct {
	Mode                  string `json:"mode"`
	LocalEnabled          bool   `json:"local_enabled"`
	Issuer                string `json:"issuer"`
	ClientID              string `json:"client_id"`
	Scopes                string `json:"scopes"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RedirectURI           string `json:"redirect_uri"`
	SiteTitle             string `json:"site_title"`
	BrandName             string `json:"brand_name"`
	BrandLogo             string `json:"brand_logo"`
	OIDCLabel             string `json:"oidc_label"`
	OIDCLogo              string `json:"oidc_logo"`
}

func buildAuthConfig(cfg config.Config, req *http.Request, settings *service.SettingsService) authConfigPayload {
	// 生效认证配置：system_configs 优先，配置文件兜底（登录页与向导页共用）
	effective := service.ResolveAuth(settings, service.EffectiveAuth{
		Issuer: cfg.OIDCIssuer, ClientID: cfg.OIDCClientID,
		Audience: cfg.OIDCAudience, Scopes: cfg.OIDCScopes,
	})
	issuer := effective.Issuer
	authEndpoint := ""
	tokenEndpoint := ""
	if issuer != "" && effective.ClientID != "" {
		oidc := service.NewOIDCService()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if meta, err := oidc.Discover(ctx, issuer); err == nil {
			authEndpoint = meta.AuthorizationEndpoint
			tokenEndpoint = meta.TokenEndpoint
			issuer = meta.Issuer
		}
		cancel()
	}
	payload := authConfigPayload{
		Mode:                  cfg.AuthMode,
		LocalEnabled:          effective.LocalEnabled,
		Issuer:                issuer,
		ClientID:              effective.ClientID,
		Scopes:                effective.Scopes,
		AuthorizationEndpoint: authEndpoint,
		TokenEndpoint:         tokenEndpoint,
		RedirectURI:           adminBaseURL(cfg, req) + "/auth/callback",
		SiteTitle:             "Islet 图床",
		BrandName:             "Islet",
		OIDCLabel:             "OIDC 登录",
	}
	if settings != nil {
		payload.SiteTitle = settings.Get(service.SettingSiteTitle)
		payload.BrandName = settings.Get(service.SettingBrandName)
		payload.BrandLogo = settings.Get(service.SettingBrandLogo)
		payload.OIDCLabel = settings.Get(service.SettingBrandOIDCLabel)
		payload.OIDCLogo = settings.Get(service.SettingBrandOIDCLogo)
	}
	if payload.OIDCLabel == "" {
		payload.OIDCLabel = "OIDC 登录"
	}
	return payload
}

func adminBaseURL(cfg config.Config, req *http.Request) string {
	if cfg.AdminBaseURL != "" {
		return cfg.AdminBaseURL
	}
	if req == nil {
		return ""
	}
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	if forwarded := req.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		scheme = forwarded
	}
	return scheme + "://" + req.Host
}

func handleAuthConfig(c *gin.Context, deps *Deps) {
	cfg := config.Load()
	if deps != nil {
		cfg = deps.Cfg
	}
	var settings *service.SettingsService
	if deps != nil {
		settings = deps.Settings
	}
	response.OK(c, buildAuthConfig(cfg, c.Request, settings))
}

// ---- OIDC 前端 PKCE 回调：后端代理 token 交换（规避 IdP CORS 限制） ----

func registerAuthRoutes(api *gin.RouterGroup, deps *Deps) {
	api.POST("/auth/oidc/callback", func(c *gin.Context) {
		var in struct {
			Code         string `json:"code"`
			CodeVerifier string `json:"code_verifier"`
			RedirectURI  string `json:"redirect_uri"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || in.Code == "" || in.CodeVerifier == "" {
			response.BadRequest(c, "缺少 code 或 code_verifier")
			return
		}
		cfg := config.Load()
		if deps != nil {
			cfg = deps.Cfg
		}
		effective := service.ResolveAuth(deps.Settings, service.EffectiveAuth{
			Issuer: cfg.OIDCIssuer, ClientID: cfg.OIDCClientID,
			Audience: cfg.OIDCAudience, Scopes: cfg.OIDCScopes,
		})
		if effective.Issuer == "" || effective.ClientID == "" {
			response.BadRequest(c, "当前未配置 OIDC 登录，请在系统设置中填写 Issuer 与 Client ID")
			return
		}
		meta, err := deps.OIDC.Discover(c.Request.Context(), effective.Issuer)
		if err != nil {
			response.BadRequest(c, "OIDC Discovery 失败："+err.Error())
			return
		}
		redirectURI := strings.TrimSpace(in.RedirectURI)
		if redirectURI == "" {
			redirectURI = adminBaseURL(cfg, c.Request) + "/auth/callback"
		}
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {in.Code},
			"code_verifier": {in.CodeVerifier},
			"client_id":     {effective.ClientID},
			"redirect_uri":  {redirectURI},
		}
		tokenResp, err := exchangeToken(c.Request.Context(), meta.TokenEndpoint, form)
		if err != nil {
			response.Unauthorized(c, "授权码交换失败："+err.Error())
			return
		}
		// 校验 access token 可验签且主体一致
		audience := effective.Audience
		if audience == "" {
			audience = effective.ClientID
		}
		identity, err := deps.OIDC.Verify(c.Request.Context(), tokenResp.AccessToken, service.VerifyOptions{
			Issuer: effective.Issuer, Audience: audience,
		})
		if err != nil {
			response.Unauthorized(c, "令牌验证失败："+err.Error())
			return
		}
		// 首登预建档，让管理员在用户列表立刻看到
		if deps.Auth != nil {
			if _, err := deps.Auth.ProvisionOIDC(identity); err != nil {
				deps.Logger.Warn("provision oidc user failed", "error", err)
			}
		}
		response.OK(c, gin.H{
			"access_token": tokenResp.AccessToken,
			"id_token":     tokenResp.IDToken,
			"expires_in":   tokenResp.ExpiresIn,
			"username":     identity.Username,
		})
	})

	api.POST("/auth/local/login", localLoginLimit(), func(c *gin.Context) {
		if !deps.Settings.GetBool(service.SettingAuthLocalEnabled, true) {
			response.Forbidden(c, "本地登录已关闭，请使用统一登录")
			return
		}
		var input struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			response.BadRequest(c, "username and password are required")
			return
		}
		user, token, err := deps.Auth.LoginLocal(input.Username, input.Password)
		if err != nil {
			if errors.Is(err, service.ErrAccountUnavailable) {
				response.Forbidden(c, "账号已被停用")
				return
			}
			response.Unauthorized(c, "用户名或密码错误")
			return
		}
		secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(middleware.LocalSessionCookie, token, int((12 * time.Hour).Seconds()), "/", "", secure, true)
		response.OK(c, middleware.FromModel(user))
	})

	api.POST("/auth/logout", func(c *gin.Context) {
		if token, err := c.Cookie(middleware.LocalSessionCookie); err == nil {
			_ = deps.Auth.RevokeSession(token)
		}
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(middleware.LocalSessionCookie, "", -1, "/", "", false, true)
		response.OK(c, gin.H{"logged_out": true})
	})

	api.GET("/auth/me", deps.Authenticator.Middleware(), func(c *gin.Context) {
		user, ok := middleware.CurrentUser(c)
		if !ok {
			response.Unauthorized(c, "unauthorized")
			return
		}
		response.OK(c, user)
	})
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func exchangeToken(ctx context.Context, endpoint string, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("token 响应解析失败（HTTP %d）", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK || result.AccessToken == "" {
		return nil, fmt.Errorf("token 端点返回 HTTP %d", resp.StatusCode)
	}
	return &result, nil
}

// ---- 健康检查 ----

func registerHealthRoutes(api *gin.RouterGroup) {
	api.GET("/health", func(c *gin.Context) {
		cfg := config.Load()
		response.OK(c, gin.H{
			"status":  "ok",
			"version": "0.1.0",
			"setup":   map[string]bool{"complete": cfg.Complete()},
		})
	})
}
