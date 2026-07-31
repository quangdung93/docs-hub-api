package http

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/usecase"
)

type AuthHandler struct {
	authUC usecase.AuthUseCase
}

func NewAuthHandler(authUC usecase.AuthUseCase) *AuthHandler {
	return &AuthHandler{authUC: authUC}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// EnvelopeSuccess tạo payload trả về theo chuẩn ISC Envelope
func EnvelopeSuccess(data interface{}) gin.H {
	return gin.H{
		"success": true,
		"data":    data,
		"error":   nil,
		"meta":    map[string]interface{}{},
	}
}

// EnvelopeError tạo payload lỗi theo chuẩn ISC Envelope
func EnvelopeError(errMessage string) gin.H {
	return gin.H{
		"success": false,
		"data":    nil,
		"error":   errMessage,
		"meta":    map[string]interface{}{},
	}
}

// @Summary Đăng nhập
// @Description Xác thực và trả về JWT qua Cookie
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LoginRequest true "Thông tin đăng nhập"
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Router /public/api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, EnvelopeError("Dữ liệu không hợp lệ"))
		return
	}

	user, token, err := h.authUC.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, EnvelopeError(err.Error()))
		return
	}

	// Xác định cờ secure cho cookie dựa trên môi trường
	secure := os.Getenv("ENV") == "production"

	// Thiết lập SameSite Lax và lưu JWT vào Cookie
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("access_token", token, 24*3600, "/", "", secure, true) // name, value, maxAge, path, domain, secure, httpOnly

	// Trả về cả user và token để Client có thể dùng làm Bearer header
	c.JSON(http.StatusOK, EnvelopeSuccess(gin.H{
		"user":  user,
		"token": token,
	}))
}

// @Summary Đăng xuất
// @Description Xóa token ở client (xóa Cookie) và revoke ở server
// @Tags auth
// @Produce json
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Router /internal/api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	// Lấy token từ header Bearer (theo chuẩn của Auth middleware)
	authHeader := c.GetHeader("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token := authHeader[7:]
		_ = h.authUC.Logout(c.Request.Context(), token)
	}

	// Xóa cookie ở client
	c.SetCookie("access_token", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, EnvelopeSuccess("đăng xuất thành công"))
}

// @Summary Lấy thông tin cá nhân
// @Description Lấy thông tin user hiện tại qua JWT
// @Tags auth
// @Produce json
// @Success 200 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Router /internal/api/v1/auth/me [get]
func (h *AuthHandler) GetMe(c *gin.Context) {
	// Sử dụng ActorFrom từ contextx (chuẩn của dự án)
	actor, exists := contextx.ActorFrom(c.Request.Context())
	if !exists {
		c.JSON(http.StatusUnauthorized, EnvelopeError("chưa xác thực"))
		return
	}

	user, err := h.authUC.GetMe(c.Request.Context(), actor.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, EnvelopeError("không tìm thấy người dùng"))
		return
	}

	c.JSON(http.StatusOK, EnvelopeSuccess(user))
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
