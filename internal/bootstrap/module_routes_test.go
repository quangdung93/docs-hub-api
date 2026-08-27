package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/config"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
	chathttp "github.com/quangdung93/docs-hub-api/internal/module/chat/delivery/http"
	documenthttp "github.com/quangdung93/docs-hub-api/internal/module/document/delivery/http"
	projecthttp "github.com/quangdung93/docs-hub-api/internal/module/project/delivery/http"
	retrievalhttp "github.com/quangdung93/docs-hub-api/internal/module/retrieval/delivery/http"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
)

// Các module dùng chung prefix /projects phải thống nhất tên wildcard ở từng
// segment; nếu lệch (:id và :project_id), Gin sẽ panic ngay lúc khởi động.
func TestProjectModuleRoutes_KhongXungDotWildcard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	internal := engine.Group("/internal/api/v1")

	require.NotPanics(t, func() {
		projecthttp.Register(internal, nil, nil)
		documenthttp.Register(internal, nil)
		retrievalhttp.Register(internal, nil)
		chathttp.Register(internal, nil)
	})
}

func TestMCPRoute_DungBearerJWTVaActorContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager, err := jwt.NewManager(jwt.Config{
		Algorithm: jwt.AlgHS256, Secret: "mcp-test-secret", Issuer: "test", AccessTTL: time.Minute,
	})
	require.NoError(t, err)
	token, err := manager.Sign("9d4710dd-5f62-4618-95e8-7cbd28659ca7", "mcp@test.local", nil, time.Now())
	require.NoError(t, err)

	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		actor, ok := contextx.ActorFrom(request.Context())
		require.True(t, ok)
		require.Equal(t, "mcp@test.local", actor.Email)
		w.WriteHeader(http.StatusNoContent)
	})
	cfg := &config.Config{App: config.AppConfig{Env: config.EnvProduction}}
	infra := &Infra{JWT: manager, Log: zap.NewNop()}
	require.NoError(t, registerRoutes(engine, cfg, infra, nil, handler))

	unauthorized := httptest.NewRecorder()
	engine.ServeHTTP(unauthorized,
		httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	authorized := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	engine.ServeHTTP(authorized, request)
	require.Equal(t, http.StatusNoContent, authorized.Code)
}
