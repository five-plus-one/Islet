package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"islet/backend/internal/config"
	"islet/backend/internal/database"
	"islet/backend/internal/middleware"
	"islet/backend/internal/router"
	"islet/backend/internal/service"
	"islet/backend/internal/web"
	"islet/backend/internal/worker"
)

// Runtime 已初始化的运行态：完整业务路由 + worker。
type Runtime struct {
	cfg        config.Config
	deps       *router.Deps
	public     *gin.Engine
	admin      *gin.Engine
	stopWorker func()
}

// App 进程壳：未初始化时管理端只提供 setup；完成后热切换到完整运行态。
type App struct {
	cfg    config.Config
	logger *slog.Logger

	mu      sync.RWMutex
	runtime *Runtime

	setupEngine *gin.Engine
}

func New(cfg config.Config, logger *slog.Logger) *App {
	a := &App{cfg: cfg, logger: logger}
	router.SetResetSystem(a.ResetSetup)
	a.setupEngine = a.buildSetupEngine()
	return a
}

func (a *App) current() *Runtime {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.runtime
}

func (a *App) swap(rt *Runtime) {
	a.mu.Lock()
	old := a.runtime
	a.runtime = rt
	a.mu.Unlock()
	if old != nil && old.stopWorker != nil {
		old.stopWorker()
	}
}

func (a *App) baseURL() string {
	if a.current() != nil {
		return a.current().deps.Settings.Get(service.SettingPublicBaseURL)
	}
	return ""
}

func (a *App) buildServices(cfg config.Config, db *gorm.DB, rdb *redis.Client) *router.Deps {
	settings := service.NewSettingsService(db)
	_, _ = settings.All() // 预热
	box := service.NewSecretBox(cfg.DataDir)
	oidc := service.NewOIDCService()
	auth := service.NewAuthService(db)
	tokens := service.NewAPITokenService(db)
	policies := service.NewPolicyService(db, box, a.baseURL, cfg.DataDir)
	groups := service.NewGroupService(db)
	uploads := service.NewUploadService(db, rdb, policies, groups, settings)
	images := service.NewImageService(db, policies)
	albums := service.NewAlbumService(db)
	stats := service.NewStatsService(db, uploads)
	audit := service.NewAuditService(db)
	authenticator := middleware.NewAuthenticator(func() config.Config { return cfg }, a.logger, db, oidc, tokens, settings)
	return &router.Deps{
		Cfg: cfg, Logger: a.logger, DB: db, Redis: rdb,
		Settings: settings, OIDC: oidc, Auth: auth, Tokens: tokens,
		Uploads: uploads, Images: images, Albums: albums,
		Policies: policies, Groups: groups, Stats: stats, Audit: audit,
		Authenticator: authenticator,
		DataDir:       cfg.DataDir,
	}
}

// Bootstrap 尝试进入完整运行态（启动时与初始化完成后调用）。
func (a *App) Bootstrap() error {
	cfg := config.Load()
	if !cfg.Complete() {
		return errors.New("installation incomplete")
	}
	db, err := database.ConnectMySQL(cfg.DSN())
	if err != nil {
		return fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	// Redis 可选：未配置或连接失败仅降级（上传限流），不阻塞启动
	var rdb *redis.Client
	if cfg.RedisAddr() != "" {
		var err error
		rdb, err = database.ConnectRedis(cfg.RedisAddr(), cfg.RedisPassword, cfg.RedisDB)
		if err != nil {
			a.logger.Warn("redis unavailable, rate limiting degraded", "error", err)
			rdb = nil
		}
	}
	if err := database.Migrate(db); err != nil {
		_ = closeDB(db)
		if rdb != nil {
			_ = rdb.Close()
		}
		return fmt.Errorf("数据库迁移失败: %w", err)
	}
	if err := database.Seed(db, ""); err != nil {
		_ = closeDB(db)
		if rdb != nil {
			_ = rdb.Close()
		}
		return fmt.Errorf("种子数据写入失败: %w", err)
	}

	deps := a.buildServices(cfg, db, rdb)
	rt := &Runtime{cfg: cfg, deps: deps}
	rt.public = router.BuildPublicEngine(deps)
	rt.admin = router.BuildAdminEngine(deps)
	rt.stopWorker = worker.New(db, deps.Settings, deps.Images, a.logger).Start()

	a.cfg = cfg
	a.swap(rt)
	a.logger.Info("islet ready", "public", cfg.HTTPAddr, "admin", cfg.AdminHTTPAddr)
	return nil
}

func closeDB(db *gorm.DB) error {
	if sqlDB, err := db.DB(); err == nil {
		return sqlDB.Close()
	}
	return nil
}

// CompleteSetup 初始化向导收尾：由 setup 路由在事务提交后调用。
func (a *App) CompleteSetup() error {
	if err := a.Bootstrap(); err != nil {
		return err
	}
	a.logger.Info("setup completed, business enabled in-process")
	return nil
}

// ResetSetup 恢复初始化：停业务、写 marker、配置文件回退未安装态。
func (a *App) ResetSetup() error {
	a.mu.Lock()
	old := a.runtime
	a.runtime = nil
	a.mu.Unlock()
	if old != nil && old.stopWorker != nil {
		old.stopWorker()
	}
	if old != nil && old.deps != nil {
		_ = closeDB(old.deps.DB)
	}

	cfg := config.Load()
	cfg.ResetPending = true
	cfg.InstallationComplete = false
	cfg.BootstrapVerified = false
	cfg.BootstrapSubject = ""
	cfg.BootstrapUsername = ""
	cfg.BootstrapEmail = ""
	cfg.BootstrapPassword = ""
	if err := config.SaveFile(cfg.ConfigFile, cfg); err != nil {
		return err
	}
	if err := config.ResetMarker(cfg.ConfigFile); err != nil {
		return err
	}
	a.cfg = cfg
	return nil
}

// ---- 引擎分发 ----

func (a *App) handlePublic(c *gin.Context) {
	if rt := a.current(); rt != nil {
		rt.public.ServeHTTP(c.Writer, c.Request)
		return
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"code": 4503, "message": "服务尚未初始化，请在管理端完成初始化向导"})
}

func (a *App) handleAdmin(c *gin.Context) {
	if rt := a.current(); rt != nil {
		rt.admin.ServeHTTP(c.Writer, c.Request)
		return
	}
	a.setupEngine.ServeHTTP(c.Writer, c.Request)
}

func (a *App) buildSetupEngine() *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())
	deps := &router.Deps{
		Cfg: a.cfg, Logger: a.logger,
		OIDC: service.NewOIDCService(),
		DataDir: a.cfg.DataDir,
		OnSetupComplete: a.CompleteSetup,
	}
	router.RegisterSetupRoutes(engine, deps)
	router.RegisterAuthConfigRoute(engine, deps)
	web.RegisterAdminSPA(engine)
	return engine
}

// Run 启动双端口并按安装状态 Bootstrap；阻塞直到 ctx 取消或监听失败。
func (a *App) Run(ctx context.Context) error {
	if a.cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	if err := a.Bootstrap(); err != nil {
		a.logger.Info("enter setup mode", "reason", err.Error())
	}

	publicEngine := gin.New()
	publicEngine.Use(gin.Recovery())
	publicEngine.Any("/*any", a.handlePublic)

	adminEngine := gin.New()
	adminEngine.Use(gin.Recovery())
	adminEngine.Any("/*any", a.handleAdmin)

	errCh := make(chan error, 2)
	publicServer := &http.Server{Addr: a.cfg.HTTPAddr, Handler: publicEngine, ReadHeaderTimeout: 10 * time.Second}
	adminServer := &http.Server{Addr: a.cfg.AdminHTTPAddr, Handler: adminEngine, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		a.logger.Info("public listener", "addr", a.cfg.HTTPAddr)
		if err := publicServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("public listener: %w", err)
		}
	}()
	go func() {
		a.logger.Info("admin listener", "addr", a.cfg.AdminHTTPAddr)
		if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("admin listener: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		_ = publicServer.Close()
		_ = adminServer.Close()
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = publicServer.Shutdown(shutdownCtx)
	_ = adminServer.Shutdown(shutdownCtx)
	if rt := a.current(); rt != nil {
		if rt.stopWorker != nil {
			rt.stopWorker()
		}
		if sqlDB, err := rt.deps.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	return nil
}

// EnsureDataDir 保证数据目录存在。
func EnsureDataDir(cfg config.Config) error {
	if cfg.DataDir == "" {
		return nil
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "uploads"), 0700); err != nil {
		return err
	}
	return nil
}
