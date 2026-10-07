package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config 进程内配置。基础设施（DB/Redis/OIDC/端口）只来自配置文件与环境变量，
// 业务运行时设置存 system_configs 表（见 service.SettingsService）。
type Config struct {
	AppEnv        string
	HTTPAddr      string
	AdminHTTPAddr string
	LogLevel      slog.Level
	ConfigFile    string
	DataDir       string

	MySQLHost     string
	MySQLPort     string
	MySQLDatabase string
	MySQLUser     string
	MySQLPassword string
	MySQLParams   string
	MySQLDSN      string

	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	AuthMode      string // "oidc" | "local"
	OIDCIssuer    string
	OIDCClientID  string
	OIDCAudience  string
	OIDCScopes    string
	AdminBaseURL  string

	// 初始化向导暂存（写入配置文件，complete 后清除）
	BootstrapVerified    bool
	BootstrapSubject     string
	BootstrapUsername    string
	BootstrapEmail       string
	BootstrapPassword    string // bcrypt 哈希
	ResetPending         bool
	InstallationComplete bool
}

type FileConfig struct {
	AppEnv        string `json:"app_env,omitempty"`
	MySQLHost     string `json:"mysql_host,omitempty"`
	MySQLPort     string `json:"mysql_port,omitempty"`
	MySQLDatabase string `json:"mysql_database,omitempty"`
	MySQLUser     string `json:"mysql_user,omitempty"`
	MySQLPassword string `json:"mysql_password,omitempty"`
	MySQLParams   string `json:"mysql_params,omitempty"`
	RedisHost     string `json:"redis_host,omitempty"`
	RedisPort     string `json:"redis_port,omitempty"`
	RedisPassword string `json:"redis_password,omitempty"`
	RedisDB       int    `json:"redis_db,omitempty"`
	AuthMode      string `json:"auth_mode,omitempty"`
	OIDCIssuer    string `json:"oidc_issuer,omitempty"`
	OIDCClientID  string `json:"oidc_client_id,omitempty"`
	OIDCAudience  string `json:"oidc_audience,omitempty"`
	OIDCScopes    string `json:"oidc_scopes,omitempty"`
	AdminBaseURL  string `json:"admin_base_url,omitempty"`

	BootstrapVerified bool   `json:"bootstrap_verified,omitempty"`
	BootstrapSubject  string `json:"bootstrap_subject,omitempty"`
	BootstrapUsername string `json:"bootstrap_username,omitempty"`
	BootstrapEmail    string `json:"bootstrap_email,omitempty"`
	BootstrapPassword string `json:"bootstrap_password_hash,omitempty"`

	ResetPending         bool `json:"reset_pending,omitempty"`
	InstallationComplete bool `json:"installation_complete,omitempty"`
}

func Load() Config {
	cfg := Config{
		AppEnv:        "production",
		HTTPAddr:      ":8080",
		AdminHTTPAddr: ":8081",
		LogLevel:      slog.LevelInfo,
		ConfigFile:    envString("CONFIG_FILE", "data/islet.json"),
		MySQLHost:     envString("SETUP_DEFAULT_MYSQL_HOST", "127.0.0.1"),
		MySQLPort:     "3306",
		MySQLDatabase: "islet",
		MySQLUser:     "islet",
		MySQLParams:   "charset=utf8mb4&parseTime=True&loc=Local",
		RedisHost:     envString("SETUP_DEFAULT_REDIS_HOST", ""),
		RedisPort:     "6379",
		AuthMode:      "oidc",
		OIDCScopes:    "openid profile email",
		DataDir:       "",
	}
	if persisted, err := loadFile(cfg.ConfigFile); err == nil {
		applyFile(&cfg, persisted)
	}
	applyEnvironment(&cfg)
	cfg.rebuild()
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Dir(cfg.ConfigFile)
	}
	if _, err := os.Stat(resetMarkerPath(cfg.ConfigFile)); err == nil {
		cfg.ResetPending = true
		cfg.InstallationComplete = false
	}
	return cfg
}

func (c Config) DSN() string {
	if c.MySQLDSN != "" {
		return c.MySQLDSN
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?%s",
		c.MySQLUser, c.MySQLPassword, c.MySQLHost, c.MySQLPort, c.MySQLDatabase, c.MySQLParams)
}

func (c Config) RedisAddr() string {
	return netAddr(c.RedisHost, c.RedisPort, "6379")
}

// DBReady / AuthReady / AdminVerified / Complete 描述初始化状态机各阶段。
func (c Config) DBReady() bool     { return c.MySQLHost != "" && c.MySQLUser != "" }
func (c Config) AuthReady() bool {
	if c.AuthMode == "local" {
		return true
	}
	return c.OIDCIssuer != "" && c.OIDCClientID != ""
}
func (c Config) Complete() bool { return c.InstallationComplete && !c.ResetPending }

// SaveFile 落盘配置；目录不存在时创建（0700，可能含数据库密码）。
func SaveFile(path string, cfg Config) error {
	if path == "" {
		return errors.New("config file path is empty")
	}
	payload, err := json.MarshalIndent(fileFromConfig(cfg), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0600)
}

// ResetMarker 写入后即使进程在库事务与配置切换间崩溃，重启也进入初始化态。
func ResetMarker(path string) error {
	return os.WriteFile(resetMarkerPath(path), []byte("reset"), 0600)
}

func ClearResetMarker(path string) error {
	err := os.Remove(resetMarkerPath(path))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func resetMarkerPath(path string) string { return path + ".reset" }

func loadFile(path string) (FileConfig, error) {
	var result FileConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, fmt.Errorf("parse %s: %w", path, err)
	}
	return result, nil
}

func applyFile(cfg *Config, file FileConfig) {
	setStr(&cfg.AppEnv, file.AppEnv)
	setStr(&cfg.MySQLHost, file.MySQLHost)
	setStr(&cfg.MySQLPort, file.MySQLPort)
	setStr(&cfg.MySQLDatabase, file.MySQLDatabase)
	setStr(&cfg.MySQLUser, file.MySQLUser)
	setStr(&cfg.MySQLPassword, file.MySQLPassword)
	setStr(&cfg.MySQLParams, file.MySQLParams)
	setStr(&cfg.RedisHost, file.RedisHost)
	setStr(&cfg.RedisPort, file.RedisPort)
	setStr(&cfg.RedisPassword, file.RedisPassword)
	if file.RedisDB != 0 {
		cfg.RedisDB = file.RedisDB
	}
	setStr(&cfg.AuthMode, file.AuthMode)
	setStr(&cfg.OIDCIssuer, file.OIDCIssuer)
	setStr(&cfg.OIDCClientID, file.OIDCClientID)
	setStr(&cfg.OIDCAudience, file.OIDCAudience)
	setStr(&cfg.OIDCScopes, file.OIDCScopes)
	setStr(&cfg.AdminBaseURL, file.AdminBaseURL)
	cfg.BootstrapVerified = file.BootstrapVerified
	setStr(&cfg.BootstrapSubject, file.BootstrapSubject)
	setStr(&cfg.BootstrapUsername, file.BootstrapUsername)
	setStr(&cfg.BootstrapEmail, file.BootstrapEmail)
	setStr(&cfg.BootstrapPassword, file.BootstrapPassword)
	cfg.ResetPending = file.ResetPending
	cfg.InstallationComplete = file.InstallationComplete
}

func applyEnvironment(cfg *Config) {
	setStr(&cfg.AppEnv, envString("APP_ENV", ""))
	setStr(&cfg.HTTPAddr, envString("HTTP_ADDR", ""))
	setStr(&cfg.AdminHTTPAddr, envString("ADMIN_HTTP_ADDR", ""))
	if v := envString("LOG_LEVEL", ""); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			cfg.LogLevel = slog.LevelDebug
		case "warn":
			cfg.LogLevel = slog.LevelWarn
		case "error":
			cfg.LogLevel = slog.LevelError
		}
	}
	setStr(&cfg.MySQLHost, envString("MYSQL_HOST", ""))
	setStr(&cfg.MySQLPort, envString("MYSQL_PORT", ""))
	setStr(&cfg.MySQLDatabase, envString("MYSQL_DATABASE", ""))
	setStr(&cfg.MySQLUser, envString("MYSQL_USER", ""))
	setStr(&cfg.MySQLPassword, envString("MYSQL_PASSWORD", ""))
	setStr(&cfg.RedisHost, envString("REDIS_HOST", ""))
	setStr(&cfg.RedisPort, envString("REDIS_PORT", ""))
	setStr(&cfg.RedisPassword, envString("REDIS_PASSWORD", ""))
	if v := envString("REDIS_DB", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RedisDB = n
		}
	}
	setStr(&cfg.AuthMode, envString("AUTH_MODE", ""))
	setStr(&cfg.OIDCIssuer, envString("OIDC_ISSUER", ""))
	setStr(&cfg.OIDCClientID, envString("OIDC_CLIENT_ID", ""))
	setStr(&cfg.OIDCAudience, envString("OIDC_AUDIENCE", ""))
	setStr(&cfg.OIDCScopes, envString("OIDC_SCOPES", ""))
	setStr(&cfg.AdminBaseURL, envString("ADMIN_BASE_URL", ""))
}

func (c *Config) rebuild() {
	c.MySQLHost = strings.TrimSpace(c.MySQLHost)
	c.RedisHost = strings.TrimSpace(c.RedisHost)
	c.OIDCIssuer = strings.TrimRight(strings.TrimSpace(c.OIDCIssuer), "/")
	c.AdminBaseURL = strings.TrimRight(strings.TrimSpace(c.AdminBaseURL), "/")
	if c.AuthMode != "local" && c.AuthMode != "oidc" {
		c.AuthMode = "oidc"
	}
}

func fileFromConfig(cfg Config) FileConfig {
	return FileConfig{
		AppEnv:              cfg.AppEnv,
		MySQLHost:           cfg.MySQLHost,
		MySQLPort:           cfg.MySQLPort,
		MySQLDatabase:       cfg.MySQLDatabase,
		MySQLUser:           cfg.MySQLUser,
		MySQLPassword:       cfg.MySQLPassword,
		MySQLParams:         cfg.MySQLParams,
		RedisHost:           cfg.RedisHost,
		RedisPort:           cfg.RedisPort,
		RedisPassword:       cfg.RedisPassword,
		RedisDB:             cfg.RedisDB,
		AuthMode:            cfg.AuthMode,
		OIDCIssuer:          cfg.OIDCIssuer,
		OIDCClientID:        cfg.OIDCClientID,
		OIDCAudience:        cfg.OIDCAudience,
		OIDCScopes:          cfg.OIDCScopes,
		AdminBaseURL:        cfg.AdminBaseURL,
		BootstrapVerified:   cfg.BootstrapVerified,
		BootstrapSubject:    cfg.BootstrapSubject,
		BootstrapUsername:   cfg.BootstrapUsername,
		BootstrapEmail:      cfg.BootstrapEmail,
		BootstrapPassword:   cfg.BootstrapPassword,
		ResetPending:        cfg.ResetPending,
		InstallationComplete: cfg.InstallationComplete,
	}
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func setStr(target *string, value string) {
	if strings.TrimSpace(value) != "" {
		*target = strings.TrimSpace(value)
	}
}

func netAddr(host, port, defaultPort string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.Contains(host, ":") && !strings.Contains(host[len(host)-1:], ":") {
		return host // 已含端口或为 IPv6 字面量
	}
	port = strings.TrimSpace(port)
	if port == "" {
		port = defaultPort
	}
	return host + ":" + port
}
