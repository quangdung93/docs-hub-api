package filesystem

import (
	"context"
	"fmt"
	"os"
)

// HealthChecker kiểm tra storage root local còn truy cập được.
type HealthChecker struct{ rootPath string }

func NewHealthChecker(store *Store) *HealthChecker {
	return &HealthChecker{rootPath: store.rootPath}
}

func (*HealthChecker) Name() string { return "filesystem-storage" }

func (h *HealthChecker) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(h.rootPath)
	if err != nil {
		return fmt.Errorf("kiểm tra filesystem storage: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("filesystem storage root không phải thư mục")
	}
	return nil
}
