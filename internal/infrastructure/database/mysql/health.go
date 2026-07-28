package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// healthChecker kiểm tra kết nối MySQL cho readiness probe.
type healthChecker struct {
	db *gorm.DB
}

// NewHealthChecker tạo checker sức khỏe MySQL (implement port.HealthChecker).
func NewHealthChecker(db *gorm.DB) *healthChecker {
	return &healthChecker{db: db}
}

func (h *healthChecker) Name() string { return "mysql" }

// Check ping DB với context (tôn trọng timeout của readiness).
func (h *healthChecker) Check(ctx context.Context) error {
	sqlDB, err := h.db.DB()
	if err != nil {
		return fmt.Errorf("lấy *sql.DB: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}
	return nil
}
