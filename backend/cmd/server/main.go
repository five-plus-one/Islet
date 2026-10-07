package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"islet/backend/internal/app"
	"islet/backend/internal/config"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()
	logger.Info("islet starting", "app_env", cfg.AppEnv, "config_file", cfg.ConfigFile)

	if err := app.EnsureDataDir(cfg); err != nil {
		logger.Error("prepare data dir failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.New(cfg, logger).Run(ctx); err != nil {
		logger.Error("islet exited with error", "error", err)
		os.Exit(1)
	}
	logger.Info("islet stopped")
}
