package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
)

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
