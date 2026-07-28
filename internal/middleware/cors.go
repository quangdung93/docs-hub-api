package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"document-hub-api/internal/config"
)

// CORS cấu hình CORS từ config. Whitelist origin cụ thể — KHÔNG dùng "*" khi
// allow_credentials=true (trình duyệt sẽ từ chối, và đó cũng là rủi ro bảo mật).
func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     cfg.AllowedMethods,
		AllowHeaders:     cfg.AllowedHeaders,
		ExposeHeaders:    []string{HeaderRequestID},
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           time.Duration(cfg.MaxAge) * time.Second,
	})
}
