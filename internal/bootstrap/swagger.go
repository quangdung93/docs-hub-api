package bootstrap

import (
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"

	"github.com/quangdung93/docs-hub-api/internal/config"

	// blank import để đăng ký đặc tả swagger (docs/swagger/docs.go).
	_ "github.com/quangdung93/docs-hub-api/docs/swagger"
)

// registerSwagger mount Swagger UI tại /swagger/*any — CHỈ khi bật trong config
// (mặc định tắt ở production). Đặc tả lấy từ package docs/swagger.
func registerSwagger(engine *gin.Engine, cfg *config.Config) {
	if !cfg.HTTP.EnableSwagger {
		return
	}
	swagger := engine.Group("/swagger")
	swagger.Use(relaxCSPForSwaggerUI())
	swagger.GET("/*any", ginswagger.WrapHandler(swaggerfiles.Handler))
}

// relaxCSPForSwaggerUI nới CSP mặc định "default-src 'none'" (đặt ở
// middleware.SecureHeaders, đúng cho API JSON thuần) — vì bản thân trang
// Swagger UI cần load JS/CSS/script inline của chính nó (cùng origin) và gọi
// fetch() tới doc.json. Chỉ áp dụng cho group /swagger, không ảnh hưởng API còn lại.
func relaxCSPForSwaggerUI() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"script-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		c.Next()
	}
}
