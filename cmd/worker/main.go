// Command worker xử lý ingestion jobs thành chunks và vector embeddings.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/quangdung93/docs-hub-api/internal/config"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/ai/localai"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	objectstorage "github.com/quangdung93/docs-hub-api/internal/infrastructure/storage"
	"github.com/quangdung93/docs-hub-api/internal/module/ingestion"
	"github.com/quangdung93/docs-hub-api/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "worker lỗi: %v\n", err)
		os.Exit(1)
	}
}
func run() error { //nolint:lll
	path := flag.String("config", "configs/config.local.yaml", "đường dẫn config")
	flag.Parse()
	cfg, err := config.Load(*path)
	if err != nil {
		return fmt.Errorf("nạp config: %w", err)
	}
	if cfg.LocalAI.EmbeddingModel == "" {
		return fmt.Errorf("thiếu local_ai.embedding_model")
	}
	log, err := logger.New(logger.Options{
		Level: cfg.Log.Level, Encoding: cfg.Log.Encoding,
		AppName: cfg.App.Name + "-worker", Env: string(cfg.App.Env),
	})
	if err != nil {
		return fmt.Errorf("khởi tạo logger: %w", err)
	}
	defer func() { _ = log.Sync() }()
	db, err := postgres.New(postgres.Config{
		DSN: cfg.Postgres.DSN(), MaxOpenConns: cfg.Postgres.MaxOpenConns,
		MaxIdleConns:    cfg.Postgres.MaxIdleConns,
		ConnMaxLifetime: cfg.Postgres.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Postgres.ConnMaxIdleTime,
	}, log)
	if err != nil {
		return fmt.Errorf("kết nối PostgreSQL: %w", err)
	}
	defer postgres.Close(db)
	store, _, err := objectstorage.New(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("khởi tạo object storage: %w", err)
	}
	embed := localai.New(cfg.LocalAI.BaseURL, cfg.LocalAI.EmbeddingModel, cfg.LocalAI.EmbeddingDimension, cfg.LocalAI.Timeout)
	processor := ingestion.NewProcessor(db, store, embed, ingestion.Config{
		ChunkLines: cfg.Ingestion.ChunkLines, OverlapLines: cfg.Ingestion.OverlapLines,
		BatchSize: cfg.Ingestion.BatchSize, EmbeddingModel: cfg.LocalAI.EmbeddingModel,
		EmbeddingDimension: cfg.LocalAI.EmbeddingDimension,
	})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(cfg.Ingestion.PollInterval)
	defer ticker.Stop()
	log.Info("ingestion worker đã chạy")
	for {
		processed, procErr := processor.ProcessNext(ctx)
		if procErr != nil {
			log.Error("ingestion thất bại", zap.Error(procErr))
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
