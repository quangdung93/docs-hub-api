package bootstrap

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/config"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
	userdomain "github.com/quangdung93/docs-hub-api/internal/module/user/domain"
)

// registerRoutes dựng 2 nhóm route và gắn các module.
//
//   - /internal/api/v1 : yêu cầu xác thực JWT (Auth middleware ở cấp group).
//   - /public/api/v1   : không yêu cầu xác thực (health, dev-token, profile công khai...).
//
// Theo templates/02, KHÔNG lặp prefix /api/v1 vô nghĩa; ở đây tách rõ internal vs public.
func registerRoutes(engine *gin.Engine, cfg *config.Config, infra *Infra, modules []Module) error {
	public := engine.Group("/public/api/v1")
	internal := engine.Group("/internal/api/v1")
	if cfg.App.IsLocal() {
		actor, err := loadLocalActor(infra)
		if err != nil {
			return err
		}
		internal.Use(middleware.LocalActor(actor))
		infra.Log.Warn("Đã TẮT JWT authentication cho internal API ở local",
			zap.String("actor_email", actor.Email), zap.String("actor_id", actor.UserID))
	} else {
		internal.Use(middleware.Auth(infra.JWT, infra.Cache))
	}

	// dev-token chỉ bật ở local (đã được config chặn ở môi trường khác).
	if cfg.App.EnableDevToken {
		registerDevToken(public, infra.JWT)
		infra.Log.Warn("Đã bật endpoint /public/api/v1/auth/dev-token (CHỈ dành cho local)")
	}

	for _, m := range modules {
		m.RegisterRoutes(internal, public)
	}
	return nil
}

func loadLocalActor(infra *Infra) (contextx.Actor, error) {
	var user struct {
		ID, Email string
	}
	if err := infra.DB.Table("users").Select("id,email").
		Where("email=? AND deleted_at IS NULL", userdomain.DefaultAdminEmail).Take(&user).Error; err != nil {
		return contextx.Actor{}, fmt.Errorf("không tìm thấy local actor %s; chạy `make seed`: %w",
			userdomain.DefaultAdminEmail, err)
	}
	return contextx.Actor{UserID: user.ID, Email: user.Email, Roles: []string{"admin"}}, nil
}
