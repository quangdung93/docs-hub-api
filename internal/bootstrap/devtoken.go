package bootstrap

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/common/validatorx"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
)

// devTokenRequest là body của endpoint cấp token cho môi trường local.
type devTokenRequest struct {
	Email string   `json:"email" binding:"required,email"`
	Roles []string `json:"roles" binding:"omitempty,dive,oneof=admin user"`
}

// registerDevToken gắn POST /auth/dev-token — CHỈ khi cfg.App.EnableDevToken=true
// (loader đã chặn bật ngoài local). Dùng để test các API cần JWT khi module auth
// chưa được hiện thực. Module auth thật sẽ thay thế endpoint này.
func registerDevToken(public *gin.RouterGroup, mgr *jwt.Manager) {
	public.POST("/auth/dev-token", func(c *gin.Context) {
		var req devTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			_ = c.Error(apperr.BadRequest("Dữ liệu không hợp lệ").WithDetails(validatorx.ToDetails(err)))
			return
		}
		roles := req.Roles
		if len(roles) == 0 {
			roles = []string{"admin"}
		}

		token, err := mgr.Sign(uuid.NewString(), req.Email, roles, time.Now())
		if err != nil {
			_ = c.Error(apperr.Internal("Không thể tạo token").WithCause(err))
			return
		}
		response.OK(c, gin.H{"access_token": token, "token_type": "Bearer"})
	})
}
