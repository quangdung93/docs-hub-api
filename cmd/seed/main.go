// Command seed nạp dữ liệu mẫu (idempotent). Seed là DỮ LIỆU, không phải schema
// (schema do cmd/migrate lo). Bị CHẶN chạy trên production để tránh tai nạn.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/quangdung393/docs-hub-api/internal/config"
	"github.com/quangdung393/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung393/docs-hub-api/internal/module/user/domain"
	"github.com/quangdung393/docs-hub-api/internal/module/user/repository"
	"github.com/quangdung393/docs-hub-api/pkg/hashing"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed lỗi: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.local.yaml", "đường dẫn file cấu hình")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("nạp cấu hình: %w", err)
	}
	if cfg.App.IsProduction() {
		return errors.New("từ chối seed trên môi trường production")
	}

	log := zap.NewNop()
	db, err := postgres.New(postgres.Config{
		DSN:          cfg.Postgres.DSN(),
		MaxOpenConns: cfg.Postgres.MaxOpenConns,
		MaxIdleConns: cfg.Postgres.MaxIdleConns,
	}, log)
	if err != nil {
		return fmt.Errorf("kết nối PostgreSQL: %w", err)
	}
	defer func() { _ = postgres.Close(db) }()

	return seedAdmin(context.Background(), repository.New(db), hashing.NewHasher(0))
}

// seedAdmin tạo tài khoản admin mặc định nếu chưa tồn tại (idempotent).
func seedAdmin(ctx context.Context, repo domain.UserRepository, hasher *hashing.Hasher) error {
	const adminEmail = "admin@local"

	exists, err := repo.ExistsByEmail(ctx, adminEmail, nil)
	if err != nil {
		return fmt.Errorf("kiểm tra admin tồn tại: %w", err)
	}
	if exists {
		fmt.Println("seed: admin đã tồn tại, bỏ qua")
		return nil
	}

	hash, err := hasher.Hash("Admin@12345")
	if err != nil {
		return fmt.Errorf("băm mật khẩu admin: %w", err)
	}
	admin := domain.NewUser(adminEmail, "Quản trị viên", hash, []string{"admin"})
	if err := repo.Create(ctx, admin); err != nil {
		return fmt.Errorf("tạo admin: %w", err)
	}

	fmt.Printf("seed: đã tạo admin %s / Admin@12345\n", adminEmail)
	return nil
}
