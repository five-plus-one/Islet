package worker

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"islet/backend/internal/model"
	"islet/backend/internal/service"
)

// Worker 周期任务：回收站按保留期清理、过期会话清理。
type Worker struct {
	interval time.Duration
	run      func(ctx context.Context)
}

func New(db *gorm.DB, settings *service.SettingsService, images *service.ImageService, logger *slog.Logger) *Worker {
	return &Worker{
		interval: time.Hour,
		run: func(ctx context.Context) {
			// 会话清理
			if err := db.Where("expires_at < ?", time.Now().UTC()).Delete(&model.AuthSession{}).Error; err != nil {
				logger.Warn("清理过期会话失败", "error", err)
			}
			// 回收站清理
			retention := int(settings.GetInt64(service.SettingRetentionDays, 30))
			if retention > 0 {
				n, err := images.PurgeExpired(ctx, retention)
				if err != nil {
					logger.Warn("清理回收站失败", "error", err)
				} else if n > 0 {
					logger.Info("回收站清理完成", "count", n)
				}
			}
		},
	}
}

// Start 启动循环；返回停止函数。
func (w *Worker) Start() (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.run(ctx)
			}
		}
	}()
	return cancel
}
