package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
)

// Tracing trả về middleware otelgin tạo span OpenTelemetry cho mỗi request.
// Đăng ký NGAY TRƯỚC TraceIDInjector trong chuỗi middleware.
func Tracing(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}

// TraceIDInjector rút trace_id từ span (đã được Tracing tạo) và đưa vào context
// của ta, để logger và response envelope dùng chung một trace_id.
func TraceIDInjector() gin.HandlerFunc {
	return func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		if sc := span.SpanContext(); sc.HasTraceID() {
			ctx := contextx.WithTraceID(c.Request.Context(), sc.TraceID().String())
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}
