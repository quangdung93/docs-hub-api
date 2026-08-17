package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/config"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
)

func TestSwaggerUI_ChoPhepTaiAssetVaDocJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.SecureHeaders(false))
	registerSwagger(engine, &config.Config{HTTP: config.HTTPConfig{EnableSwagger: true}})

	index := httptest.NewRecorder()
	engine.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))
	require.Equal(t, http.StatusOK, index.Code)
	require.Contains(t, index.Body.String(), "swagger-ui-bundle.js")
	require.Contains(t, index.Header().Get("Content-Security-Policy"), "script-src 'self' 'unsafe-inline'")

	asset := httptest.NewRecorder()
	engine.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/swagger/swagger-ui-bundle.js", nil))
	require.Equal(t, http.StatusOK, asset.Code)
	require.Contains(t, asset.Header().Get("Content-Type"), "javascript")

	document := httptest.NewRecorder()
	engine.ServeHTTP(document, httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil))
	require.Equal(t, http.StatusOK, document.Code)
	require.True(t, strings.Contains(document.Body.String(), `"/internal/api/v1/projects/{project_id}/documents"`))
}
