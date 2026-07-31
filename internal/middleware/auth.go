package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	jwt5 "github.com/golang-jwt/jwt/v5"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
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
			// Fallback: Thử đọc token từ Cookie "access_token"
			cookieToken, errCookie := c.Cookie("access_token")
			if errCookie != nil || cookieToken == "" {
				abortWith(c, apperr.Unauthorized("Thiếu hoặc sai định dạng token"))
				return
			}
			raw = cookieToken
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

// EnvelopeError tạo payload lỗi theo chuẩn ISC Envelope
func EnvelopeError(errMessage string) gin.H {
	return gin.H{
		"success": false,
		"data":    nil,
		"error":   errMessage,
		"meta":    map[string]interface{}{},
	}
}

// RequireAuth middleware dùng để xác thực JWT token từ cookie
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Lấy token từ cookie "access_token"
		tokenString, err := c.Cookie("access_token")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, EnvelopeError("không tìm thấy access token"))
			return
		}

		secret := os.Getenv("JWT_SECRET")

		// Giải mã và verify JWT
		token, err := jwt5.Parse(tokenString, func(token *jwt5.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt5.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("phương thức ký không hợp lệ")
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, EnvelopeError("token không hợp lệ hoặc đã hết hạn"))
			return
		}

		// Trích xuất thông tin user từ claims
		claims, ok := token.Claims.(jwt5.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, EnvelopeError("không thể đọc payload của token"))
			return
		}

		userID, ok := claims["user_id"].(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, EnvelopeError("thiếu user_id trong token"))
			return
		}

		role, ok := claims["role"].(string)
		if !ok {
			role = "" // Role fallback
		}

		// Gán thông tin user vào context để sử dụng ở handler tiếp theo
		c.Set("user_id", userID)
		c.Set("role", role)

		c.Next()
	}
}
