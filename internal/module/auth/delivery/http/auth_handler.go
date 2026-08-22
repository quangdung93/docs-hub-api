// Package http là tầng delivery của module auth: bind/validate request, gọi
// usecase, trả envelope. Handler phải MỎNG — không chứa business logic.
package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/common/validatorx"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/usecase"
)

// Tên cookie dùng chung cho cả lúc đặt và lúc xóa.
const (
	accessCookie  = "access_token"
	refreshCookie = "refresh_token"
)

// AuthHandler xử lý HTTP cho module auth. Chỉ giữ tham chiếu usecase — không state khác.
type AuthHandler struct {
	authUC usecase.AuthUseCase
	// secureCookie bật cờ Secure của cookie. Lấy từ config (env production/staging)
	// thay vì đọc biến môi trường trực tiếp trong handler.
	secureCookie bool
}

// NewAuthHandler tạo AuthHandler.
func NewAuthHandler(authUC usecase.AuthUseCase, secureCookie bool) *AuthHandler {
	return &AuthHandler{authUC: authUC, secureCookie: secureCookie}
}

// UserResponse là thông tin user trả cho client.
//
// roles là MẢNG, không phải chuỗi JSON — trả thẳng domain.User sẽ khiến
// client phải parse hai lần (mục #13 trong báo cáo API).
type UserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	FullName  string    `json:"full_name"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u *domain.User) *UserResponse {
	if u == nil {
		return nil
	}
	return &UserResponse{
		ID:        u.ID.String(),
		Username:  u.Username,
		FullName:  u.FullName,
		Roles:     u.RolesList(),
		CreatedAt: u.CreatedAt,
	}
}

// LoginRequest là body đăng nhập.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RefreshRequest là body gia hạn token.
//
// Bỏ trống thì handler đọc refresh token từ cookie — hỗ trợ cả client dùng
// header lẫn client dùng cookie.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutRequest là body đăng xuất (không bắt buộc).
//
// Có refresh_token thì chỉ thu hồi đúng phiên đó; bỏ trống thì thu hồi MỌI
// phiên của user (đăng xuất khỏi mọi thiết bị).
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Login godoc
// @Summary  Đăng nhập
// @Description Xác thực và trả về access token + refresh token (kèm cookie)
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

	user, pair, err := h.authUC.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		_ = c.Error(apperr.Unauthorized("Tên đăng nhập hoặc mật khẩu không đúng"))
		return
	}

	h.setAuthCookies(c, pair)
	// Giữ nguyên khóa "token" để client cũ không vỡ; bổ sung refresh_token.
	response.OK(c, gin.H{
		"user":          toUserResponse(user),
		"token":         pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
	})
}

// Refresh godoc
// @Summary  Gia hạn access token
// @Description Đổi refresh token còn hiệu lực lấy cặp token mới. Refresh token cũ bị thu hồi ngay.
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body  body      RefreshRequest  false  "Refresh token (bỏ trống thì đọc từ cookie)"
// @Success  200   {object}  response.Envelope
// @Failure  401   {object}  response.Envelope
// @Router   /public/api/v1/auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshRequest
	// Body rỗng là hợp lệ (client dùng cookie) nên bỏ qua lỗi bind.
	_ = c.ShouldBindJSON(&req)

	token := req.RefreshToken
	if token == "" {
		token, _ = c.Cookie(refreshCookie)
	}

	user, pair, err := h.authUC.Refresh(c.Request.Context(), token)
	if err != nil {
		h.clearAuthCookies(c)
		_ = c.Error(apperr.Unauthorized("Refresh token không hợp lệ hoặc đã hết hạn"))
		return
	}

	h.setAuthCookies(c, pair)
	response.OK(c, gin.H{
		"user":          toUserResponse(user),
		"token":         pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
	})
}

// Logout godoc
// @Summary  Đăng xuất
// @Description Thu hồi session ở server và xóa cookie. Không kèm refresh_token thì thu hồi mọi phiên của user.
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body  body      LogoutRequest  false  "Refresh token cần thu hồi (bỏ trống = mọi phiên)"
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	actor, ok := contextx.ActorFrom(c.Request.Context())
	if !ok {
		_ = c.Error(apperr.Unauthorized("Chưa xác thực"))
		return
	}
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		_ = c.Error(apperr.Unauthorized("Chưa xác thực"))
		return
	}

	var req LogoutRequest
	_ = c.ShouldBindJSON(&req)
	if req.RefreshToken == "" {
		req.RefreshToken, _ = c.Cookie(refreshCookie)
	}

	// Lấy chính access token đang dùng để vô hiệu nó ngay, không đợi hết hạn.
	accessToken := bearerToken(c.GetHeader("Authorization"))
	if accessToken == "" {
		accessToken, _ = c.Cookie(accessCookie)
	}

	_ = h.authUC.Logout(c.Request.Context(), userID, req.RefreshToken, accessToken)

	h.clearAuthCookies(c)
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

	response.OK(c, toUserResponse(user))
}

// bearerToken tách token khỏi header "Authorization: Bearer <token>".
// Trả chuỗi rỗng nếu header thiếu hoặc sai định dạng.
func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

// setAuthCookies đặt cookie cho cả hai token. Thời hạn cookie lấy ĐÚNG theo TTL
// của token bên trong — trước đây cookie sống 24h trong khi token chỉ 15 phút,
// khiến client tưởng còn đăng nhập nhưng mọi request đều 401.
func (h *AuthHandler) setAuthCookies(c *gin.Context, pair usecase.TokenPair) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(accessCookie, pair.AccessToken, pair.ExpiresIn, "/", "", h.secureCookie, true)
	// Refresh token chỉ cần gửi tới endpoint refresh/logout, nhưng để path "/"
	// cho đơn giản; httpOnly để JavaScript không đọc được.
	c.SetCookie(refreshCookie, pair.RefreshToken, pair.RefreshExpiresIn, "/", "", h.secureCookie, true)
}

func (h *AuthHandler) clearAuthCookies(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(accessCookie, "", -1, "/", "", h.secureCookie, true)
	c.SetCookie(refreshCookie, "", -1, "/", "", h.secureCookie, true)
}

// Register gắn các endpoint của Auth vào Gin Router.
// Lưu ý: "Register" ở đây là thuật ngữ đăng ký route (đường dẫn API),
// KHÔNG PHẢI là chức năng Đăng ký tài khoản (User Registration).
func Register(internal, public *gin.RouterGroup, h *AuthHandler) {
	// public route (không cần token) — refresh phải public vì access token đã hết hạn.
	public.POST("/auth/login", h.Login)
	public.POST("/auth/refresh", h.Refresh)

	// internal route (cần token)
	authGroup := internal.Group("/auth")
	{
		authGroup.POST("/logout", h.Logout)
		authGroup.GET("/me", h.GetMe)
	}
}
