package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain/mocks"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
)

const (
	testAccessTTL  = 15 * time.Minute
	testRefreshTTL = 168 * time.Hour
)

func setupTest(t *testing.T) (*mocks.MockUserRepository, *mocks.MockSessionRepository, AuthUseCase) {
	userRepo := mocks.NewMockUserRepository(t)
	sessionRepo := mocks.NewMockSessionRepository(t)
	mgr, _ := jwt.NewManager(jwt.Config{
		Secret:    "test-secret",
		Issuer:    "test-issuer",
		AccessTTL: testAccessTTL,
	})
	uc := NewAuthUseCase(userRepo, sessionRepo, mgr, testAccessTTL, testRefreshTTL)
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
			mockSetup: func(ur *mocks.MockUserRepository, _ *mocks.MockSessionRepository) {
				ur.On("FindByUsername", mock.Anything, "wronguser").Return(nil, errors.New("not found")).Once()
			},
			expectedError: ErrInvalidCredentials,
		},
		{
			name:     "Fail_WrongPassword",
			username: "testuser",
			password: "wrong_password",
			mockSetup: func(ur *mocks.MockUserRepository, _ *mocks.MockSessionRepository) {
				ur.On("FindByUsername", mock.Anything, "testuser").Return(validUser, nil).Once()
			},
			expectedError: ErrInvalidCredentials,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ur, sr, uc := setupTest(t)
			tt.mockSetup(ur, sr)

			user, pair, err := uc.Login(t.Context(), tt.username, tt.password)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Empty(t, pair.AccessToken)
				assert.Empty(t, pair.RefreshToken)
				assert.Nil(t, user)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, user)
			assert.NotEmpty(t, pair.AccessToken)
			assert.NotEmpty(t, pair.RefreshToken)
			assert.Empty(t, user.PasswordHash)
			// TTL phải lấy từ config, không hardcode.
			assert.Equal(t, int(testAccessTTL.Seconds()), pair.ExpiresIn)
			assert.Equal(t, int(testRefreshTTL.Seconds()), pair.RefreshExpiresIn)
		})
	}
}

// TestLogin_LuuRefreshTokenChuKhongPhaiAccessToken chốt quyết định thiết kế:
// bảng sessions giữ refresh token để thu hồi được, không giữ access token.
func TestLogin_LuuRefreshTokenChuKhongPhaiAccessToken(t *testing.T) {
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.DefaultCost)
	validUser := &domain.User{ID: uuid.New(), Username: "u", PasswordHash: string(hashedPassword)}

	ur, sr, uc := setupTest(t)
	ur.On("FindByUsername", mock.Anything, "u").Return(validUser, nil).Once()

	var saved *domain.Session
	sr.On("Create", mock.Anything, mock.AnythingOfType("*domain.Session")).
		Run(func(args mock.Arguments) { saved = args.Get(1).(*domain.Session) }).
		Return(nil).Once()

	_, pair, err := uc.Login(t.Context(), "u", "pw")
	require.NoError(t, err)
	require.NotNil(t, saved)

	assert.Equal(t, pair.RefreshToken, saved.Token, "session phải lưu refresh token")
	assert.NotEqual(t, pair.AccessToken, saved.Token, "session KHÔNG được lưu access token")
	assert.Equal(t, validUser.ID, saved.UserID)
	// Hạn session phải theo refreshTTL, sai số nhỏ do thời gian chạy.
	assert.WithinDuration(t, time.Now().Add(testRefreshTTL), saved.ExpiresAt, time.Minute)
}

func TestAuthUseCase_Refresh(t *testing.T) {
	userID := uuid.New()
	validUser := &domain.User{ID: userID, Username: "testuser", PasswordHash: "hash"}

	t.Run("Success_CapTokenMoiVaThuHoiTokenCu", func(t *testing.T) {
		ur, sr, uc := setupTest(t)
		const oldToken = "old-refresh-token"

		sr.On("FindByToken", mock.Anything, oldToken).Return(&domain.Session{
			ID: uuid.New(), UserID: userID, Token: oldToken,
			ExpiresAt: time.Now().Add(time.Hour),
		}, nil).Once()
		ur.On("FindByID", mock.Anything, userID.String()).Return(validUser, nil).Once()
		sr.On("Create", mock.Anything, mock.AnythingOfType("*domain.Session")).Return(nil).Once()
		sr.On("Delete", mock.Anything, oldToken).Return(nil).Once()

		user, pair, err := uc.Refresh(t.Context(), oldToken)

		require.NoError(t, err)
		require.NotNil(t, user)
		assert.NotEmpty(t, pair.AccessToken)
		assert.NotEqual(t, oldToken, pair.RefreshToken, "phải xoay vòng sang refresh token mới")
		assert.Empty(t, user.PasswordHash)
	})

	t.Run("Fail_TokenRong", func(t *testing.T) {
		_, _, uc := setupTest(t)
		_, _, err := uc.Refresh(t.Context(), "")
		assert.ErrorIs(t, err, ErrInvalidRefresh)
	})

	t.Run("Fail_KhongTonTai", func(t *testing.T) {
		_, sr, uc := setupTest(t)
		sr.On("FindByToken", mock.Anything, "unknown").Return(nil, domain.ErrSessionNotFound).Once()

		_, _, err := uc.Refresh(t.Context(), "unknown")
		assert.ErrorIs(t, err, ErrInvalidRefresh)
	})

	t.Run("Fail_HetHan_VaDonBanGhiRac", func(t *testing.T) {
		_, sr, uc := setupTest(t)
		const expired = "expired-token"

		sr.On("FindByToken", mock.Anything, expired).Return(&domain.Session{
			ID: uuid.New(), UserID: userID, Token: expired,
			ExpiresAt: time.Now().Add(-time.Minute), // đã hết hạn
		}, nil).Once()
		// Bản ghi hết hạn phải bị xóa chứ không để lại rác.
		sr.On("Delete", mock.Anything, expired).Return(nil).Once()

		_, _, err := uc.Refresh(t.Context(), expired)
		assert.ErrorIs(t, err, ErrInvalidRefresh)
	})

	t.Run("Fail_UserDaBiXoa_ThiThuHoiSession", func(t *testing.T) {
		ur, sr, uc := setupTest(t)
		const token = "orphan-token"

		sr.On("FindByToken", mock.Anything, token).Return(&domain.Session{
			ID: uuid.New(), UserID: userID, Token: token,
			ExpiresAt: time.Now().Add(time.Hour),
		}, nil).Once()
		ur.On("FindByID", mock.Anything, userID.String()).Return(nil, errors.New("not found")).Once()
		sr.On("Delete", mock.Anything, token).Return(nil).Once()

		_, _, err := uc.Refresh(t.Context(), token)
		assert.ErrorIs(t, err, ErrInvalidRefresh)
	})
}

func TestAuthUseCase_Logout(t *testing.T) {
	userID := uuid.New()

	t.Run("CoRefreshToken_ThiChiThuHoiPhienDo", func(t *testing.T) {
		_, sr, uc := setupTest(t)
		sr.On("Delete", mock.Anything, "some-token").Return(nil).Once()

		assert.NoError(t, uc.Logout(t.Context(), userID, "some-token"))
	})

	t.Run("KhongCoRefreshToken_ThiThuHoiMoiPhien", func(t *testing.T) {
		_, sr, uc := setupTest(t)
		sr.On("DeleteByUserID", mock.Anything, userID).Return(nil).Once()

		assert.NoError(t, uc.Logout(t.Context(), userID, ""))
	})
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

		require.NoError(t, err)
		require.NotNil(t, user)
		assert.Empty(t, user.PasswordHash)
	})

	t.Run("Fail_NotFound", func(t *testing.T) {
		ur.On("FindByID", mock.Anything, "not_found").Return(nil, errors.New("not found")).Once()

		user, err := uc.GetMe(context.Background(), "not_found")

		assert.Error(t, err)
		assert.Nil(t, user)
	})
}

// TestNewRefreshToken_KhongTrungNhau đảm bảo token sinh ra là ngẫu nhiên thật.
func TestNewRefreshToken_KhongTrungNhau(t *testing.T) {
	seen := make(map[string]bool, 100)
	for range 100 {
		tok, err := newRefreshToken()
		require.NoError(t, err)
		assert.NotEmpty(t, tok)
		assert.False(t, seen[tok], "refresh token bị trùng: %s", tok)
		seen[tok] = true
	}
}
