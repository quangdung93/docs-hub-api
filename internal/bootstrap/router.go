package bootstrap

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/config"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
	userdomain "github.com/quangdung93/docs-hub-api/internal/module/user/domain"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
)

// registerRoutes dựng 2 nhóm route và gắn các module.
//
//   - /internal/api/v1 : yêu cầu xác thực JWT (Auth middleware ở cấp group).
//   - /public/api/v1   : không yêu cầu xác thực (health, dev-token, profile công khai...).
//
// Theo templates/02, KHÔNG lặp prefix /api/v1 vô nghĩa; ở đây tách rõ internal vs public.
func registerRoutes(
	engine *gin.Engine, cfg *config.Config, infra *Infra, modules []Module, mcpHandler http.Handler,
) error {
	public := engine.Group("/public/api/v1")
	internal := engine.Group("/internal/api/v1")
	var authenticated gin.HandlerFunc
	if cfg.App.IsLocal() {
		actor, err := loadLocalActor(infra)
		if err != nil {
			return err
		}
		authenticated = middleware.LocalActor(actor)
		infra.Log.Warn("Đã TẮT JWT authentication cho internal API ở local",
			zap.String("actor_email", actor.Email), zap.String("actor_id", actor.UserID))
	} else {
		authenticated = middleware.Auth(infra.JWT, infra.Cache)
	}
	internal.Use(authenticated)

	// JWKS công bố khóa CÔNG KHAI để dịch vụ khác tự verify chữ ký token mà
	// không cần biết khóa riêng. Chỉ có ý nghĩa với RS256 — HS256 dùng secret
	// đối xứng, công bố ra là trao luôn quyền ký token.
	//
	// Đặt ở gốc theo quy ước RFC 8615 (/.well-known/...) để client chuẩn tự tìm được.
	if infra.JWT.Algorithm() == jwt.AlgRS256 {
		engine.GET("/.well-known/jwks.json", func(c *gin.Context) {
			// Trả trực tiếp, KHÔNG bọc envelope ISC: JWKS là chuẩn công khai
			// (RFC 7517), thư viện của bên gọi mong đợi đúng hình dạng {"keys":[...]}.
			c.JSON(200, infra.JWT.PublicJWKS()) //nolint:forbidigo // JWKS theo chuẩn RFC 7517
		})
		infra.Log.Info("Đã bật JWKS tại /.well-known/jwks.json")
	}

	// dev-token chỉ bật ở local (đã được config chặn ở môi trường khác).
	if cfg.App.EnableDevToken {
		registerDevToken(public, infra.JWT)
		infra.Log.Warn("Đã bật endpoint /public/api/v1/auth/dev-token (CHỈ dành cho local)")
	}

	for _, m := range modules {
		m.RegisterRoutes(internal, public)
	}
	if mcpHandler != nil {
		mcpGroup := engine.Group("/mcp")
		mcpGroup.Use(authenticated)
		mcpGroup.Any("", gin.WrapH(mcpHandler))
		infra.Log.Info("Đã bật MCP Streamable HTTP tại /mcp")
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
