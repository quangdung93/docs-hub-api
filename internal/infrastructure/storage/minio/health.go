package minio

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
)

// healthChecker kiểm tra MinIO cho readiness bằng cách kiểm tra bucket tồn tại.
type healthChecker struct {
	client *minio.Client
	bucket string
}

// NewHealthChecker tạo checker sức khỏe MinIO (implement port.HealthChecker).
func NewHealthChecker(client *minio.Client, bucket string) *healthChecker {
	return &healthChecker{client: client, bucket: bucket}
}

func (h *healthChecker) Name() string { return "minio" }

func (h *healthChecker) Check(ctx context.Context) error {
	if _, err := h.client.BucketExists(ctx, h.bucket); err != nil {
		return fmt.Errorf("kiểm tra bucket minio: %w", err)
	}
	return nil
}
