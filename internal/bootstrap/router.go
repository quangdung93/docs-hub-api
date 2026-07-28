package bootstrap

import (
	"github.com/gin-gonic/gin"

	"document-hub-api/internal/config"
	"document-hub-api/internal/middleware"
)

// registerRoutes dựng 2 nhóm route và gắn các module.
//
//   - /internal/api/v1 : yêu cầu xác thực JWT (Auth middleware ở cấp group).
//   - /public/api/v1   : không yêu cầu xác thực (health, dev-token, profile công khai...).
//
// Theo templates/02, KHÔNG lặp prefix /api/v1 vô nghĩa; ở đây tách rõ internal vs public.
func registerRoutes(engine *gin.Engine, cfg *config.Config, infra *Infra, modules []Module) {
	public := engine.Group("/public/api/v1")
	internal := engine.Group("/internal/api/v1")
	internal.Use(middleware.Auth(infra.JWT))

	// dev-token chỉ bật ở local (đã được config chặn ở môi trường khác).
	if cfg.App.EnableDevToken {
		registerDevToken(public, infra.JWT)
		infra.Log.Warn("Đã bật endpoint /public/api/v1/auth/dev-token (CHỈ dành cho local)")
	}

	for _, m := range modules {
		m.RegisterRoutes(internal, public)
	}
}
