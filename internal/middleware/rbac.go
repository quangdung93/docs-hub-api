package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/quangdung393/docs-hub-api/internal/common/apperr"
	"github.com/quangdung393/docs-hub-api/internal/common/contextx"
)

// RequireRoles chặn request nếu actor không có ÍT NHẤT MỘT trong các role yêu cầu.
// Phải đặt SAU Auth (cần actor trong context). Không đủ quyền -> AUTH_403.
func RequireRoles(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := contextx.ActorFrom(c.Request.Context())
		if !ok {
			// Không có actor nghĩa là chưa qua Auth -> coi như chưa xác thực.
			abortWith(c, apperr.Unauthorized("Chưa xác thực"))
			return
		}

		for _, r := range roles {
			if actor.HasRole(r) {
				c.Next()
				return
			}
		}
		abortWith(c, apperr.Forbidden("Bạn không có quyền thực hiện thao tác này"))
	}
}
