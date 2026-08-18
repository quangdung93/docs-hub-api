package http_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
	authhttp "github.com/quangdung93/docs-hub-api/internal/module/auth/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/usecase/mocks"
)

// fakeAuth mô phỏng middleware.Auth thành công: đặt actor cố định vào context
// mà không cần verify JWT thật — dùng riêng cho test handler.
func fakeAuth(userID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor := contextx.Actor{UserID: userID}
		c.Request = c.Request.WithContext(contextx.WithActor(c.Request.Context(), actor))
		c.Next()
	}
}

// setupRouter dựng router thật (đúng chuỗi middleware + Register) để test cả
// handler LẪN việc ErrorHandler ghi đúng envelope ISC.
func setupRouter(t *testing.T, actorUserID string) (*mocks.MockAuthUseCase, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	uc := mocks.NewMockAuthUseCase(t)
	h := authhttp.NewAuthHandler(uc)

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ErrorHandler())
	if actorUserID != "" {
		r.Use(fakeAuth(actorUserID))
	}
	authhttp.Register(r.Group("/internal/api/v1"), r.Group("/public/api/v1"), h)
	return uc, r
}

func decodeEnvelope(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env map[string]any
	require.NoError(t, json.Unmarshal(body, &env))
	return env
}

func TestLogin_Success_Returns200(t *testing.T) {
	uc, r := setupRouter(t, "")
	mockUser := &domain.User{ID: uuid.New(), Username: "test"}
	uc.On("Login", mock.Anything, "test", "123").Return(mockUser, "test_token", nil).Once()

	body, _ := json.Marshal(map[string]any{"username": "test", "password": "123"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/public/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Set-Cookie"), "access_token=test_token")

	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, true, env["success"])
	require.Nil(t, env["error"])
	meta := env["meta"].(map[string]any)
	require.NotEmpty(t, meta["request_id"], "meta phải có request_id theo chuẩn ISC")
	require.NotEmpty(t, meta["timestamp"], "meta phải có timestamp theo chuẩn ISC")
}

func TestLogin_ValidationError_Returns400(t *testing.T) {
	_, r := setupRouter(t, "")

	body, _ := json.Marshal(map[string]any{"username": "test"}) // thiếu password
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/public/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "REQ_400", errObj["code"], "error phải là OBJECT có code, không phải string trần")
	require.IsType(t, "", errObj["message"])
}

func TestLogin_InvalidCredentials_Returns401(t *testing.T) {
	uc, r := setupRouter(t, "")
	uc.On("Login", mock.Anything, "test", "wrong").Return(nil, "", errors.New("sai mật khẩu")).Once()

	body, _ := json.Marshal(map[string]any{"username": "test", "password": "wrong"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/public/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "AUTH_401", errObj["code"])
}

func TestLogout_Success_Returns200(t *testing.T) {
	uc, r := setupRouter(t, "")
	uc.On("Logout", mock.Anything, "valid_token").Return(nil).Once()

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/internal/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer valid_token")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Set-Cookie"), "Max-Age=0") // Go MaxAge=-1 render thành Max-Age=0

	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, true, env["success"])
}

func TestGetMe_Success_Returns200(t *testing.T) {
	userID := uuid.New()
	uc, r := setupRouter(t, userID.String())
	mockUser := &domain.User{ID: userID, Username: "test"}
	uc.On("GetMe", mock.Anything, userID.String()).Return(mockUser, nil).Once()

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/api/v1/auth/me", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, true, env["success"])
}

func TestGetMe_Unauthenticated_Returns401(t *testing.T) {
	_, r := setupRouter(t, "") // không gắn actor -> mô phỏng chưa qua Auth

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/api/v1/auth/me", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	errObj := env["error"].(map[string]any)
	require.Equal(t, "AUTH_401", errObj["code"])
}

func TestGetMe_NotFound_Returns404(t *testing.T) {
	userID := uuid.New()
	uc, r := setupRouter(t, userID.String())
	uc.On("GetMe", mock.Anything, userID.String()).Return(nil, errors.New("không tìm thấy user")).Once()

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/api/v1/auth/me", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	errObj := env["error"].(map[string]any)
	require.Equal(t, "USR_404", errObj["code"])
}
