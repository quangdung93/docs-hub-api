// Command api là entrypoint của HTTP API service document-hub-api.
//
// @title        document-hub-api
// @version      1.0
// @description  Boilerplate Go Clean Architecture cho ISC.
// @BasePath     /
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"document-hub-api/internal/bootstrap"
	"document-hub-api/internal/config"
	"document-hub-api/pkg/logger"
)

// version được set qua -ldflags khi build.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "khởi động thất bại: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", defaultConfigPath(), "đường dẫn file cấu hình")
	showVersion := flag.Bool("version", false, "in phiên bản rồi thoát")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("nạp cấu hình: %w", err)
	}

	log, err := logger.New(logger.Options{
		Level:    cfg.Log.Level,
		Encoding: cfg.Log.Encoding,
		AppName:  cfg.App.Name,
		Env:      string(cfg.App.Env),
	})
	if err != nil {
		return fmt.Errorf("khởi tạo logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	// Bắt SIGINT/SIGTERM để graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return bootstrap.Run(ctx, cfg, log)
}

// defaultConfigPath chọn file cấu hình theo APP_ENV (mặc định local).
func defaultConfigPath() string {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local"
	}
	return fmt.Sprintf("configs/config.%s.yaml", env)
}
