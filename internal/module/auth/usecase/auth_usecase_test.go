package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"

	"time"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
)

func setupTest(t *testing.T) (*mocks.MockUserRepository, *mocks.MockSessionRepository, AuthUseCase) {
	userRepo := mocks.NewMockUserRepository(t)
	sessionRepo := mocks.NewMockSessionRepository(t)
	mgr, _ := jwt.NewManager(jwt.Config{
		Secret:    "test-secret",
		Issuer:    "test-issuer",
		AccessTTL: 15 * time.Minute,
	})
	uc := NewAuthUseCase(userRepo, sessionRepo, mgr)
	return userRepo, sessionRepo, uc
}

func TestAuthUseCase_Login(t *testing.T) {
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("correct_password"), bcrypt.DefaultCost)
	validUser := &domain.User{
		ID:           uuid.New(),
		Username:     "testuser",
		PasswordHash: string(hashedPassword),
	}

	tests := []struct {
		name          string
		username      string
		password      string
		mockSetup     func(userRepo *mocks.MockUserRepository, sessionRepo *mocks.MockSessionRepository)
		expectedError error
	}{
		{
			name:     "Success",
			username: "testuser",
			password: "correct_password",
			mockSetup: func(ur *mocks.MockUserRepository, sr *mocks.MockSessionRepository) {
				ur.On("FindByUsername", mock.Anything, "testuser").Return(validUser, nil).Once()
				sr.On("Create", mock.Anything, mock.AnythingOfType("*domain.Session")).Return(nil).Once()
			},
			expectedError: nil,
		},
		{
			name:     "Fail_UserNotFound",
			username: "wronguser",
			password: "correct_password",
			mockSetup: func(ur *mocks.MockUserRepository, sr *mocks.MockSessionRepository) {
				ur.On("FindByUsername", mock.Anything, "wronguser").Return(nil, errors.New("not found")).Once()
			},
			expectedError: ErrInvalidCredentials,
		},
		{
			name:     "Fail_WrongPassword",
			username: "testuser",
			password: "wrong_password",
			mockSetup: func(ur *mocks.MockUserRepository, sr *mocks.MockSessionRepository) {
				ur.On("FindByUsername", mock.Anything, "testuser").Return(validUser, nil).Once()
			},
			expectedError: ErrInvalidCredentials,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ur, sr, uc := setupTest(t)
			tt.mockSetup(ur, sr)

			user, token, err := uc.Login(context.Background(), tt.username, tt.password)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Empty(t, token)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, token)
				assert.NotNil(t, user)
				assert.Empty(t, user.PasswordHash)
			}
		})
	}
}

func TestAuthUseCase_Logout(t *testing.T) {
	ur, sr, uc := setupTest(t)
	token := "dummy_token"
	_ = ur

	sr.On("Delete", mock.Anything, token).Return(nil).Once()

	err := uc.Logout(context.Background(), token)
	assert.NoError(t, err)
}

func TestAuthUseCase_GetMe(t *testing.T) {
	ur, _, uc := setupTest(t)
	userID := uuid.New()
	validUser := &domain.User{
		ID:           userID,
		Username:     "testuser",
		PasswordHash: "hashed_pass",
	}

	t.Run("Success", func(t *testing.T) {
		ur.On("FindByID", mock.Anything, userID.String()).Return(validUser, nil).Once()

		user, err := uc.GetMe(context.Background(), userID.String())

		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Empty(t, user.PasswordHash)
	})

	t.Run("Fail_NotFound", func(t *testing.T) {
		ur.On("FindByID", mock.Anything, "not_found").Return(nil, errors.New("not found")).Once()

		user, err := uc.GetMe(context.Background(), "not_found")

		assert.Error(t, err)
		assert.Nil(t, user)
	})
}
