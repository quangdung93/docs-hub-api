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
	engine.GET("/swagger/*any", swaggerHeaders(), ginswagger.WrapHandler(swaggerfiles.Handler))
}

// swaggerHeaders nới CSP đúng phạm vi Swagger UI. Trang này dùng script/style
// inline của gin-swagger; các API còn lại vẫn giữ default-src 'none'.
func swaggerHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Writer.Header()
		header.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; img-src 'self' data:; "+
				"font-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		header.Set("Cache-Control", "no-store")
		c.Next()
	}
}
