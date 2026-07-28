package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
	userhttp "github.com/quangdung93/docs-hub-api/internal/module/user/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/user/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/user/usecase"
)

// fakeService là stub của usecase.Service để test handler độc lập.
type fakeService struct {
	createFn func(ctx context.Context, in usecase.CreateInput) (*domain.User, error)
	getFn    func(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

func (f fakeService) Create(ctx context.Context, in usecase.CreateInput) (*domain.User, error) {
	return f.createFn(ctx, in)
}
func (f fakeService) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return f.getFn(ctx, id)
}
func (f fakeService) List(context.Context, usecase.ListInput) ([]domain.User, pagination.Meta, error) {
	return nil, pagination.Meta{}, nil
}
func (f fakeService) Update(context.Context, usecase.UpdateInput) (*domain.User, error) {
	return nil, nil
}
func (f fakeService) SetStatus(context.Context, usecase.SetStatusInput) (*domain.User, error) {
	return nil, nil
}
func (f fakeService) Delete(context.Context, uuid.UUID, int) error { return nil }
func (f fakeService) EmailAvailable(context.Context, string) (bool, error) {
	return true, nil
}

func setupHandler(svc usecase.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ErrorHandler())
	userhttp.Register(r.Group("/internal/api/v1"), userhttp.NewHandler(svc))
	return r
}

func TestCreateHandler_Success_Returns201WithEnvelope(t *testing.T) {
	svc := fakeService{createFn: func(_ context.Context, in usecase.CreateInput) (*domain.User, error) {
		return domain.NewUser(in.Email, in.FullName, "hash", in.Roles), nil
	}}
	r := setupHandler(svc)

	body, _ := json.Marshal(map[string]any{
		"email": "a@b.com", "full_name": "A", "password": "P@ssword1",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/api/v1/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, true, env["success"])
	data := env["data"].(map[string]any)
	require.Equal(t, "a@b.com", data["email"])
	_, hasHash := data["password_hash"]
	require.False(t, hasHash, "KHÔNG được lộ password_hash")
	require.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

func TestCreateHandler_ValidationError_Returns400(t *testing.T) {
	r := setupHandler(fakeService{})

	body, _ := json.Marshal(map[string]any{"email": "khong-phai-email"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/api/v1/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "REQ_400", errObj["code"])
}

func TestCreateHandler_DuplicateEmail_Returns200Business(t *testing.T) {
	svc := fakeService{createFn: func(context.Context, usecase.CreateInput) (*domain.User, error) {
		return nil, domain.ErrDuplicateEmail
	}}
	r := setupHandler(svc)

	body, _ := json.Marshal(map[string]any{
		"email": "dup@b.com", "full_name": "A", "password": "P@ssword1",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/api/v1/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Điểm mấu chốt chuẩn ISC: lỗi nghiệp vụ -> HTTP 200.
	require.Equal(t, http.StatusOK, w.Code)
	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "DUPLICATE_EMAIL", errObj["code"])
}
