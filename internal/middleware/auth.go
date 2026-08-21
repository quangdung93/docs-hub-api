package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/common/tokenrevoke"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
	"github.com/quangdung93/docs-hub-api/pkg/logger"
)

// TokenVerifier là interface tối thiểu mà Auth cần — cho phép test dễ và tránh
// phụ thuộc trực tiếp kiểu *jwt.Manager.
type TokenVerifier interface {
	Verify(token string) (*jwt.Claims, error)
}

// Auth xác thực Bearer JWT. Thành công -> đặt actor vào context. Thất bại ->
// AUTH_401 qua ErrorHandler.
//
// revoked là danh sách access token đã logout (có thể nil). JWT xác thực bằng
// chữ ký nên không tự thu hồi được — không tra danh sách này thì logout xong
// token cũ vẫn dùng được tới lúc hết hạn.
//
// Redis lỗi -> FAIL-OPEN (cho request đi qua) + log, giống RateLimit: sự cố
// Redis không được phép làm sập toàn bộ xác thực. Cửa sổ rủi ro tối đa bằng
// đúng thời hạn access token.
func Auth(verifier TokenVerifier, revoked port.Cache) gin.HandlerFunc {
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

		if isRevoked(c, revoked, raw) {
			abortWith(c, apperr.Unauthorized("Token đã bị thu hồi"))
			return
		}

		actor := contextx.Actor{UserID: claims.UserID, Email: claims.Email, Roles: claims.Roles}
		c.Request = c.Request.WithContext(contextx.WithActor(c.Request.Context(), actor))
		c.Next()
	}
}

// LocalActor bỏ qua JWT và gắn actor cố định đã được bootstrap đọc từ DB.
// Chỉ bootstrap local được phép dùng middleware này.
func LocalActor(actor contextx.Actor) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(contextx.WithActor(c.Request.Context(), actor))
		c.Next()
	}
}

// isRevoked tra danh sách token đã logout. Lỗi hạ tầng -> trả false (fail-open).
func isRevoked(c *gin.Context, revoked port.Cache, token string) bool {
	if revoked == nil {
		return false
	}
	ctx := c.Request.Context()
	_, err := revoked.Get(ctx, tokenrevoke.Key(token))
	switch {
	case err == nil:
		return true // có trong danh sách = đã thu hồi
	case errors.Is(err, port.ErrCacheMiss):
		return false
	default:
		logger.FromContext(ctx).Error("kiểm tra thu hồi token fail-open do lỗi backend",
			zap.Error(err))
		return false
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
