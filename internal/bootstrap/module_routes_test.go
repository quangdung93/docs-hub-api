package bootstrap

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	documenthttp "github.com/quangdung93/docs-hub-api/internal/module/document/delivery/http"
	projecthttp "github.com/quangdung93/docs-hub-api/internal/module/project/delivery/http"
	retrievalhttp "github.com/quangdung93/docs-hub-api/internal/module/retrieval/delivery/http"
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
	})
}
