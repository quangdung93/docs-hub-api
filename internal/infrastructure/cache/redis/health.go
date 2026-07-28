package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

// healthChecker kiểm tra kết nối Redis cho readiness probe.
type healthChecker struct {
	client *goredis.Client
}

// NewHealthChecker tạo checker sức khỏe Redis (implement port.HealthChecker).
func NewHealthChecker(client *goredis.Client) *healthChecker {
	return &healthChecker{client: client}
}

func (h *healthChecker) Name() string { return "redis" }

func (h *healthChecker) Check(ctx context.Context) error {
	if err := h.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	return nil
}
