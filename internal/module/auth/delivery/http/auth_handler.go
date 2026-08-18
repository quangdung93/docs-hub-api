// Package http là tầng delivery của module auth: bind/validate request, gọi
// usecase, trả envelope. Handler phải MỎNG — không chứa business logic.
package http

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/common/validatorx"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/usecase"
)

// AuthHandler xử lý HTTP cho module auth. Chỉ giữ tham chiếu usecase — không state khác.
type AuthHandler struct {
	authUC usecase.AuthUseCase
}

// NewAuthHandler tạo AuthHandler.
func NewAuthHandler(authUC usecase.AuthUseCase) *AuthHandler {
	return &AuthHandler{authUC: authUC}
}

// LoginRequest là body đăng nhập.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login godoc
// @Summary  Đăng nhập
// @Description Xác thực và trả về JWT qua Cookie
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body  body      LoginRequest  true  "Thông tin đăng nhập"
// @Success  200   {object}  response.Envelope
// @Failure  400   {object}  response.Envelope
// @Failure  401   {object}  response.Envelope
// @Router   /public/api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperr.BadRequest("Dữ liệu không hợp lệ").WithDetails(validatorx.ToDetails(err)))
		return
	}

	user, token, err := h.authUC.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		_ = c.Error(apperr.Unauthorized("Tên đăng nhập hoặc mật khẩu không đúng"))
		return
	}

	// Xác định cờ secure cho cookie dựa trên môi trường.
	secure := os.Getenv("ENV") == "production"

	// Thiết lập SameSite Lax và lưu JWT vào Cookie.
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("access_token", token, 24*3600, "/", "", secure, true) // name, value, maxAge, path, domain, secure, httpOnly

	// Trả về cả user và token để client có thể dùng làm Bearer header.
	response.OK(c, gin.H{"user": user, "token": token})
}

// Logout godoc
// @Summary  Đăng xuất
// @Description Xóa token ở client (xóa Cookie) và revoke ở server
// @Tags     auth
// @Produce  json
// @Success  200 {object} response.Envelope
// @Failure  400 {object} response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	// Lấy token từ header Bearer (theo chuẩn của Auth middleware).
	authHeader := c.GetHeader("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token := authHeader[7:]
		_ = h.authUC.Logout(c.Request.Context(), token)
	}

	// Xóa cookie ở client.
	c.SetCookie("access_token", "", -1, "/", "", false, true)
	response.OK(c, gin.H{"message": "Đăng xuất thành công"})
}

// GetMe godoc
// @Summary  Lấy thông tin cá nhân
// @Description Lấy thông tin user hiện tại qua JWT
// @Tags     auth
// @Produce  json
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  404 {object} response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/auth/me [get]
func (h *AuthHandler) GetMe(c *gin.Context) {
	actor, ok := contextx.ActorFrom(c.Request.Context())
	if !ok {
		_ = c.Error(apperr.Unauthorized("Chưa xác thực"))
		return
	}

	user, err := h.authUC.GetMe(c.Request.Context(), actor.UserID)
	if err != nil {
		_ = c.Error(apperr.NotFound(errcode.UserNotFound, "Không tìm thấy người dùng"))
		return
	}

	response.OK(c, user)
}

// Register gắn các endpoint của Auth vào Gin Router.
// Lưu ý: "Register" ở đây là thuật ngữ đăng ký route (đường dẫn API),
// KHÔNG PHẢI là chức năng Đăng ký tài khoản (User Registration).
func Register(internal, public *gin.RouterGroup, h *AuthHandler) {
	// public route (không cần token)
	public.POST("/auth/login", h.Login)

	// internal route (cần token)
	authGroup := internal.Group("/auth")
	{
		authGroup.POST("/logout", h.Logout)
		authGroup.GET("/me", h.GetMe)
	}
}
