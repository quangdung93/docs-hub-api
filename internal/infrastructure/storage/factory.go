// Package storage chọn object storage adapter theo cấu hình môi trường.
package storage

import (
	"context"
	"fmt"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/config"
	fsstore "github.com/quangdung93/docs-hub-api/internal/infrastructure/storage/filesystem"
	miniostore "github.com/quangdung93/docs-hub-api/internal/infrastructure/storage/minio"
)

// New dựng object store và readiness checker tương ứng với storage.driver.
func New(ctx context.Context, cfg *config.Config) (port.ObjectStore, port.HealthChecker, error) {
	switch cfg.Storage.Driver {
	case "filesystem":
		store, err := fsstore.New(cfg.Storage.Filesystem.Root)
		if err != nil {
			return nil, nil, fmt.Errorf("khởi tạo filesystem storage: %w", err)
		}
		return store, fsstore.NewHealthChecker(store), nil
	case "minio":
		client, err := miniostore.New(ctx, miniostore.Config{
			Endpoint: cfg.MinIO.Endpoint, AccessKey: cfg.MinIO.AccessKey,
			SecretKey: cfg.MinIO.SecretKey, Bucket: cfg.MinIO.Bucket,
			UseSSL: cfg.MinIO.UseSSL, Region: cfg.MinIO.Region,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("khởi tạo MinIO: %w", err)
		}
		return miniostore.NewStore(client, cfg.MinIO.Bucket), miniostore.NewHealthChecker(client, cfg.MinIO.Bucket), nil
	default:
		return nil, nil, fmt.Errorf("storage.driver %q không được hỗ trợ", cfg.Storage.Driver)
	}
}
