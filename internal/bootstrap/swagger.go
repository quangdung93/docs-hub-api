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
	engine.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))
}
