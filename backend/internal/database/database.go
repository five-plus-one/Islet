package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"islet/backend/internal/model"
)

// ConnectMySQL 打开连接并做带超时的 Ping 探测（初始化向导与启动自检共用）。
func ConnectMySQL(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func ConnectRedis(addr, password string, db int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db, DialTimeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(model.AllModels()...)
}

// Seed 写入内置数据：分组、本地存储策略、系统配置默认值。幂等。
func Seed(db *gorm.DB, defaultPublicBaseURL string) error {
	groups := []model.UserGroup{
		{ID: model.GroupAdministrators, Name: "administrators", Description: "内置管理员组：不限配额，可用全部策略", QuotaBytes: 0, MaxFileBytes: 0, UploadsPerHour: 0, BuiltIn: true, Status: model.StatusActive},
		{ID: model.GroupMembers, Name: "members", Description: "内置默认组：新用户默认分组", QuotaBytes: 1 << 30, MaxFileBytes: 20 << 20, AllowedMimes: []string{"image/*"}, UploadsPerHour: 100, BuiltIn: true, Status: model.StatusActive},
	}
	for i := range groups {
		if err := db.Create(&groups[i]).Error; err != nil {
			exists := db.FirstOrCreate(&groups[i], model.UserGroup{ID: groups[i].ID}).Error
			if exists != nil {
				return fmt.Errorf("seed user_groups: %w", err)
			}
		}
	}

	var policyCount int64
	if err := db.Model(&model.StoragePolicy{}).Count(&policyCount).Error; err != nil {
		return err
	}
	if policyCount == 0 {
		if err := db.Create(&model.StoragePolicy{
			ID: model.PolicyLocalDefault, Name: "本地存储", Driver: model.PolicyDriverLocal,
			Config: "{}", URLMode: model.PolicyURLDirect, Naming: model.NamingDate, IsDefault: true, Status: model.StatusActive,
		}).Error; err != nil {
			return fmt.Errorf("seed storage_policies: %w", err)
		}
	}

	defaults := map[string]string{
		"site.title":             "Islet 图床",
		"site.public_base_url":   defaultPublicBaseURL,
		"users.auto_approve":     "false",
		"recycle.retention_days": "30",
		"upload.max_bytes":       "104857600",
	}
	for key, value := range defaults {
		var count int64
		if err := db.Model(&model.SystemConfig{}).Where("key_name = ?", key).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := db.Create(&model.SystemConfig{KeyName: key, Value: value}).Error; err != nil {
				return fmt.Errorf("seed system_configs: %w", err)
			}
		}
	}
	return nil
}
