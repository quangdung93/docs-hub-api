package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"document-hub-api/internal/common/apperr"
	"document-hub-api/internal/common/contextx"
	"document-hub-api/pkg/jwt"
)

// TokenVerifier là interface tối thiểu mà Auth cần — cho phép test dễ và tránh
// phụ thuộc trực tiếp kiểu *jwt.Manager.
type TokenVerifier interface {
	Verify(token string) (*jwt.Claims, error)
}

// Auth xác thực Bearer JWT. Thành công -> đặt actor vào context. Thất bại ->
// AUTH_401 qua ErrorHandler.
func Auth(verifier TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := bearerToken(c.GetHeader("Authorization"))
		if err != nil {
			abortWith(c, apperr.Unauthorized("Thiếu hoặc sai định dạng token"))
			return
		}

		claims, err := verifier.Verify(raw)
		if err != nil {
			msg := "Token không hợp lệ"
			if errors.Is(err, jwt.ErrExpiredToken) {
				msg = "Token đã hết hạn"
			}
			abortWith(c, apperr.Unauthorized(msg))
			return
		}

		actor := contextx.Actor{UserID: claims.UserID, Email: claims.Email, Roles: claims.Roles}
		c.Request = c.Request.WithContext(contextx.WithActor(c.Request.Context(), actor))
		c.Next()
	}
}

// bearerToken tách token khỏi header "Authorization: Bearer <token>".
func bearerToken(header string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", errors.New("thiếu prefix Bearer")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", errors.New("token rỗng")
	}
	return token, nil
}

// abortWith đẩy lỗi vào gin và abort để ErrorHandler ghi envelope.
func abortWith(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}
