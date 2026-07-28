package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"document-hub-api/internal/infrastructure/telemetry"
)

// Metrics ghi nhận số lượng và thời gian xử lý request.
//
// Dùng c.FullPath() (route pattern, ví dụ "/users/:id") thay vì URL thật để
// tránh nổ cardinality nhãn Prometheus.
func Metrics(m *telemetry.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		m.IncInFlight()
		defer m.DecInFlight()

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unmatched" // route không khớp (404) -> gom về 1 nhãn
		}
		m.ObserveHTTP(c.Request.Method, path, strconv.Itoa(c.Writer.Status()), time.Since(start))
	}
}
