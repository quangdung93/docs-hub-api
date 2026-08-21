package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
)

func generateTestJWT(secret string, userID string, expired bool) string {
	expirationTime := time.Now().Add(24 * time.Hour)
	if expired {
		expirationTime = time.Now().Add(-24 * time.Hour)
	}
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    "user",
		"exp":     expirationTime.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

func TestLocalActor_KhongCanAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	actor := contextx.Actor{UserID: "local-user", Email: "admin@local", Roles: []string{"admin"}}
	engine := gin.New()
	engine.Use(LocalActor(actor))
	engine.GET("/internal", func(c *gin.Context) {
		got, ok := contextx.ActorFrom(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, actor, got)
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/internal", nil))
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("JWT_SECRET", "test-secret")
	defer os.Unsetenv("JWT_SECRET")

	t.Run("Case Pass", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		userID := "123e4567-e89b-12d3-a456-426614174000"
		validToken := generateTestJWT("test-secret", userID, false)

		c.Request = httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)
		c.Request.AddCookie(&http.Cookie{Name: "access_token", Value: validToken})

		RequireAuth()(c)

		assert.False(t, c.IsAborted())
		val, exists := c.Get("user_id")
		assert.True(t, exists)
		assert.Equal(t, userID, val)
	})

	t.Run("Case Fail (401) No Cookie", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)

		RequireAuth()(c)

		assert.True(t, c.IsAborted())
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Case Fail (401) Expired Token", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		expiredToken := generateTestJWT("test-secret", "123", true)

		c.Request = httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)
		c.Request.AddCookie(&http.Cookie{Name: "access_token", Value: expiredToken})

		RequireAuth()(c)

		assert.True(t, c.IsAborted())
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Case Fail (401) Wrong Signature", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		invalidToken := generateTestJWT("wrong-secret", "123", false)

		c.Request = httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)
		c.Request.AddCookie(&http.Cookie{Name: "access_token", Value: invalidToken})

		RequireAuth()(c)

		assert.True(t, c.IsAborted())
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
