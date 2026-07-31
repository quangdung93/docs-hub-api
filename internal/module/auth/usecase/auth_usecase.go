package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("tên đăng nhập hoặc mật khẩu không hợp lệ")
	ErrInternalError      = errors.New("lỗi hệ thống")
)

type AuthUseCase interface {
	Login(ctx context.Context, username, password string) (*domain.User, string, error)
	Logout(ctx context.Context, token string) error
	GetMe(ctx context.Context, userID string) (*domain.User, error)
}

type authUseCase struct {
	userRepo    domain.UserRepository
	sessionRepo domain.SessionRepository
	jwtManager  *jwt.Manager
}

func NewAuthUseCase(ur domain.UserRepository, sr domain.SessionRepository, jm *jwt.Manager) AuthUseCase {
	return &authUseCase{
		userRepo:    ur,
		sessionRepo: sr,
		jwtManager:  jm,
	}
}

func (u *authUseCase) Login(ctx context.Context, username, password string) (*domain.User, string, error) {
	// Lấy user từ DB
	user, err := u.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, "", ErrInvalidCredentials
	}

	// So khớp mật khẩu
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	// Giải nén JSON roles
	var roles []string
	if user.Roles != "" {
		_ = json.Unmarshal([]byte(user.Roles), &roles)
	}

	// Tạo JWT token thông qua Manager chuẩn của dự án (đã cấu hình Issuer và thời gian)
	now := time.Now()
	tokenString, err := u.jwtManager.Sign(user.ID.String(), user.Username, roles, now)
	if err != nil {
		return nil, "", ErrInternalError
	}
	
	expirationTime := now.Add(24 * time.Hour) // fallback nếu cần lưu vào DB Session

	// Lưu session vào database
	session := &domain.Session{
		ID:        uuid.New(),
		UserID:    user.ID,
		Token:     tokenString,
		ExpiresAt: expirationTime,
		CreatedAt: time.Now(),
	}

	if err := u.sessionRepo.Create(ctx, session); err != nil {
		return nil, "", ErrInternalError
	}

	// Đảm bảo không trả về thông tin nhạy cảm
	user.PasswordHash = ""

	return user, tokenString, nil
}

func (u *authUseCase) Logout(ctx context.Context, token string) error {
	return u.sessionRepo.Delete(ctx, token)
}

func (u *authUseCase) GetMe(ctx context.Context, userID string) (*domain.User, error) {
	user, err := u.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Đảm bảo không trả về thông tin nhạy cảm
	user.PasswordHash = ""

	return user, nil
}
