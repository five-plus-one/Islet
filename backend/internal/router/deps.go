package router

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"islet/backend/internal/config"
	"islet/backend/internal/middleware"
	"islet/backend/internal/service"
	"islet/backend/internal/web"
)

// Deps 路由层依赖。未初始化时 DB/Redis/业务服务为 nil，只有 setup 引擎使用部分字段。
type Deps struct {
	Cfg      config.Config
	Logger   *slog.Logger
	DB       *gorm.DB
	Redis    *redis.Client
	Settings *service.SettingsService
	OIDC     *service.OIDCService
	Auth     *service.AuthService
	Tokens   *service.APITokenService
	Uploads  *service.UploadService
	Images   *service.ImageService
	Albums   *service.AlbumService
	Policies *service.PolicyService
	Groups   *service.GroupService
	Stats    *service.StatsService
	Audit    *service.AuditService

	Authenticator *middleware.Authenticator
	DataDir       string

	// OnSetupComplete 初始化完成后由 app 调用（热启用业务）。
	OnSetupComplete func() error
}

// BuildPublicEngine 公网端口：图片访问 /i /t、Token 上传、健康检查。
func BuildPublicEngine(deps *Deps) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())
	api := engine.Group("/api/v1")
	registerHealthRoutes(api)
	registerPublicUpload(api, deps)
	registerImageRoutes(engine, deps)
	return engine
}

// BuildAdminEngine 管理端口：SPA、业务 API、管理 API。
func BuildAdminEngine(deps *Deps) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())

	api := engine.Group("/api/v1")
	registerHealthRoutes(api)
	RegisterAuthConfigRoute(engine, deps)
	registerAuthRoutes(api, deps)
	registerPublicUpload(api, deps)

	user := api.Group("")
	user.Use(deps.Authenticator.Middleware())
	registerMeRoutes(user, deps)
	registerImageManageRoutes(user, deps)
	registerAlbumRoutes(user, deps)

	admin := engine.Group("/api/admin")
	admin.Use(deps.Authenticator.Middleware())
	registerPolicyRoutes(admin, deps)
	registerGroupRoutes(admin, deps)
	registerUserRoutes(admin, deps)
	registerStatsRoutes(admin, deps)
	registerSettingsRoutes(admin, deps)
	registerSystemRoutes(admin, deps)

	web.RegisterAdminSPA(engine)
	return engine
}

// RegisterSetupRoutes 初始化向导（挂在 setup 引擎与管理引擎均可）。
func RegisterSetupRoutes(engine *gin.Engine, deps *Deps) {
	setup := engine.Group("/api/setup")
	registerSetupRoutes(setup, deps)
}

// RegisterAuthConfigRoute 认证配置（登录页与向导页免鉴权读取）。
func RegisterAuthConfigRoute(engine *gin.Engine, deps *Deps) {
	engine.GET("/api/v1/auth/config", func(c *gin.Context) {
		handleAuthConfig(c, deps)
	})
}
