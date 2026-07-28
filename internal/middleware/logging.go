package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"document-hub-api/internal/common/contextx"
	"document-hub-api/pkg/logger"
)

// Logging tạo child logger kèm request_id + trace_id, đưa vào context (để mọi
// tầng dưới dùng chung), và ghi access log sau khi request hoàn tất.
//
// Đặt sau RequestID + TraceIDInjector để có đủ 2 id.
func Logging(base *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		ctx := c.Request.Context()

		reqLogger := base.With(
			zap.String("request_id", contextx.RequestID(ctx)),
			zap.String("trace_id", contextx.TraceID(ctx)),
		)
		c.Request = c.Request.WithContext(logger.WithContext(ctx, reqLogger))

		c.Next()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.Int("bytes", c.Writer.Size()),
			zap.String("client_ip", c.ClientIP()),
		}
		if actor, ok := contextx.ActorFrom(c.Request.Context()); ok {
			fields = append(fields, zap.String("user_id", actor.UserID))
		}

		reqLogger.Info("access", fields...)
	}
}
