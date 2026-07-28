package middleware

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/quangdung393/docs-hub-api/internal/common/apperr"
	"github.com/quangdung393/docs-hub-api/internal/common/contextx"
	"github.com/quangdung393/docs-hub-api/internal/common/port"
	"github.com/quangdung393/docs-hub-api/pkg/logger"
)

// RateLimiterDeps là phụ thuộc của rate limit middleware.
type RateLimiterDeps struct {
	Cache             port.Cache
	RequestsPerWindow int
	Window            time.Duration
	// OnFallback được gọi khi limiter fail-open (backend lỗi), để tăng metric.
	OnFallback func()
}

// RateLimit giới hạn số request theo cửa sổ thời gian cố định, đếm trên Redis.
//
// Khóa theo user_id nếu đã xác thực, ngược lại theo IP. Nếu Redis lỗi -> FAIL-OPEN
// (cho request đi qua) + log + tăng metric, để sự cố Redis không làm sập toàn bộ traffic.
func RateLimit(deps RateLimiterDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		key := rateLimitKey(c)

		count, err := deps.Cache.Incr(ctx, key, deps.Window)
		if err != nil {
			logger.FromContext(ctx).Error("rate limiter fail-open do lỗi backend", zap.Error(err))
			if deps.OnFallback != nil {
				deps.OnFallback()
			}
			c.Next()
			return
		}

		if count > int64(deps.RequestsPerWindow) {
			retryAfter := int(deps.Window.Seconds())
			c.Writer.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			abortWith(c, apperr.TooManyRequests("Bạn đã gửi quá nhiều yêu cầu, vui lòng thử lại sau"))
			return
		}
		c.Next()
	}
}

// rateLimitKey chọn khóa đếm: ưu tiên user_id, fallback IP.
func rateLimitKey(c *gin.Context) string {
	if actor, ok := contextx.ActorFrom(c.Request.Context()); ok && actor.UserID != "" {
		return fmt.Sprintf("ratelimit:user:%s", actor.UserID)
	}
	return fmt.Sprintf("ratelimit:ip:%s", c.ClientIP())
}
