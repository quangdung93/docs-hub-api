package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/usecase/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func setupHandler(t *testing.T) (*mocks.MockAuthUseCase, *AuthHandler) {
	uc := mocks.NewMockAuthUseCase(t)
	return uc, NewAuthHandler(uc)
}

func TestAuthHandler_Login(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Case 200 OK", func(t *testing.T) {
		uc, h := setupHandler(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		reqBody := `{"username":"test","password":"123"}`
		c.Request = httptest.NewRequest("POST", "/api/login", bytes.NewBufferString(reqBody))
		c.Request.Header.Set("Content-Type", "application/json")

		mockUser := &domain.User{ID: uuid.New(), Username: "test"}
		uc.On("Login", mock.Anything, "test", "123").Return(mockUser, "test_token", nil).Once()

		h.Login(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Set-Cookie"), "access_token=test_token")

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.True(t, resp["success"].(bool))
	})

	t.Run("Case 400 Bad Request", func(t *testing.T) {
		_, h := setupHandler(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		reqBody := `{"username":"test"}` // missing password
		c.Request = httptest.NewRequest("POST", "/api/login", bytes.NewBufferString(reqBody))
		c.Request.Header.Set("Content-Type", "application/json")

		h.Login(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAuthHandler_Logout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Case 200 OK", func(t *testing.T) {
		uc, h := setupHandler(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("POST", "/api/logout", nil)
		c.Request.Header.Set("Authorization", "Bearer valid_token")

		uc.On("Logout", mock.Anything, "valid_token").Return(nil).Once()

		h.Logout(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Set-Cookie"), "Max-Age=0") // Go MaxAge=-1 renders as Max-Age=0
	})
}

func TestAuthHandler_GetMe(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Case 200 OK", func(t *testing.T) {
		uc, h := setupHandler(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		userID := uuid.New().String()
		
		c.Request = httptest.NewRequest("GET", "/api/me", nil)
		actor := contextx.Actor{UserID: userID}
		c.Request = c.Request.WithContext(contextx.WithActor(c.Request.Context(), actor))

		mockUser := &domain.User{ID: uuid.MustParse(userID), Username: "test"}
		uc.On("GetMe", mock.Anything, userID).Return(mockUser, nil).Once()

		h.GetMe(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.True(t, resp["success"].(bool))
	})
}
