// Package httpserver dựng gin engine + các HTTP server (API và Admin) kèm
// graceful shutdown. Đây là tầng hạ tầng — biết về gin, nhưng không chứa
// business logic.
package httpserver

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/quangdung93/docs-hub-api/internal/config"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/telemetry"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
)

// EngineDeps là phụ thuộc để dựng gin engine cho API server.
type EngineDeps struct {
	Config  *config.Config
	Logger  *zap.Logger
	Metrics *telemetry.Metrics
	// Extra là các middleware toàn cục cần hạ tầng (ví dụ RateLimit cần Redis),
	// được bootstrap chèn vào sau các middleware cốt lõi.
	Extra []gin.HandlerFunc
}

// NewAPIEngine dựng gin engine với chuỗi middleware toàn cục theo đúng thứ tự
// (xem ADR-0002). Thứ tự này quan trọng: sai là hỏng envelope/log/timeout.
func NewAPIEngine(deps EngineDeps) *gin.Engine {
	if deps.Config.App.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	// Tin proxy nội bộ; ở production nên cấu hình cụ thể qua SetTrustedProxies.
	_ = engine.SetTrustedProxies(nil)

	cfg := deps.Config

	engine.Use(
		middleware.Recovery(),                            // 1. ngoài cùng, bắt mọi panic
		middleware.RequestID(),                           // 2. sinh/đọc X-Request-ID
		middleware.Tracing(cfg.App.Name),                 // 3. tạo span OTel
		middleware.TraceIDInjector(),                     // 3b. đưa trace_id vào context
		middleware.Logging(deps.Logger),                  // 4. child logger + access log
		middleware.Metrics(deps.Metrics),                 // 5. Prometheus
		middleware.ErrorHandler(),                        // 6. điểm DUY NHẤT ghi lỗi
		middleware.SecureHeaders(cfg.App.IsProduction()), // 7. security headers
		middleware.CORS(cfg.CORS),                        // 8. CORS
		middleware.BodyLimit(cfg.HTTP.MaxBodyBytes),      // 9. giới hạn body
	)

	// 10. RateLimit (cần Redis) do bootstrap chèn qua Extra.
	engine.Use(deps.Extra...)

	// 11. Timeout xử lý mỗi request.
	engine.Use(middleware.Timeout(cfg.Timeout.Handler))

	return engine
}
