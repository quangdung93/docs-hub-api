package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
)

func setupRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.GET("/test", handler)
	return r
}

func doRequest(t *testing.T, r *gin.Engine) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w, body
}

// TestBusinessErrorAlwaysReturns200 — bất biến quan trọng nhất của chuẩn ISC.
func TestBusinessErrorAlwaysReturns200(t *testing.T) {
	r := setupRouter(func(c *gin.Context) {
		be := apperr.NewBusiness(errcode.DuplicateEmail, "Email đã tồn tại", false)
		_ = c.Error(be)
	})

	w, body := doRequest(t, r)

	require.Equal(t, http.StatusOK, w.Code, "lỗi nghiệp vụ PHẢI trả HTTP 200")
	require.Equal(t, false, body["success"])
	require.Nil(t, body["data"])
	errObj := body["error"].(map[string]any)
	require.Equal(t, errcode.DuplicateEmail, errObj["code"])
}

// TestTechnicalErrorNeverReturns200 — kỹ thuật không bao giờ được ẩn dưới 200.
func TestTechnicalErrorNeverReturns200(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"bad request", apperr.BadRequest("thiếu field"), http.StatusBadRequest, errcode.Req400},
		{"unauthorized", apperr.Unauthorized("thiếu token"), http.StatusUnauthorized, errcode.Auth401},
		{"forbidden", apperr.Forbidden("không đủ quyền"), http.StatusForbidden, errcode.Auth403},
		{"not found", apperr.NotFound(errcode.UserNotFound, "không thấy user"), http.StatusNotFound, errcode.UserNotFound},
		{"internal", apperr.Internal("crash"), http.StatusInternalServerError, errcode.Sys500},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := setupRouter(func(c *gin.Context) { _ = c.Error(tc.err) })
			w, body := doRequest(t, r)

			require.Equal(t, tc.wantStatus, w.Code)
			require.NotEqual(t, http.StatusOK, w.Code, "lỗi kỹ thuật KHÔNG được trả 200")
			errObj := body["error"].(map[string]any)
			require.Equal(t, tc.wantCode, errObj["code"])
		})
	}
}

// TestUnknownErrorBecomesSys500WithGenericMessage — không lộ chi tiết nội bộ.
func TestUnknownErrorBecomesSys500WithGenericMessage(t *testing.T) {
	r := setupRouter(func(c *gin.Context) {
		_ = c.Error(errRaw("chi tiết nhạy cảm: connection refused 10.0.0.5"))
	})
	w, body := doRequest(t, r)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	errObj := body["error"].(map[string]any)
	require.Equal(t, errcode.Sys500, errObj["code"])
	require.NotContains(t, errObj["message"], "10.0.0.5", "không được lộ chi tiết nội bộ ra client")
}

type errRaw string

func (e errRaw) Error() string { return string(e) }
