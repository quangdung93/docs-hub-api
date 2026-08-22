package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/common/tokenrevoke"
	"github.com/quangdung93/docs-hub-api/pkg/logger"

	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
)

var (
	ErrInvalidCredentials = errors.New("tên đăng nhập hoặc mật khẩu không hợp lệ")
	ErrInvalidRefresh     = errors.New("refresh token không hợp lệ hoặc đã hết hạn")
	ErrInternalError      = errors.New("lỗi hệ thống")
)

// refreshTokenBytes là số byte ngẫu nhiên của refresh token (256 bit).
const refreshTokenBytes = 32

// TokenPair là cặp token trả về sau khi đăng nhập hoặc gia hạn.
//
// AccessToken là JWT ngắn hạn, xác thực bằng chữ ký nên KHÔNG lưu database.
// RefreshToken là chuỗi ngẫu nhiên dài hạn, BẮT BUỘC lưu database để thu hồi
// được — đó là lý do nó không phải JWT: JWT đã ký thì không rút lại được.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	// ExpiresIn là số giây còn lại của AccessToken, để client biết khi nào cần gia hạn.
	ExpiresIn int
	// RefreshExpiresIn là số giây còn lại của RefreshToken (dùng đặt hạn cookie).
	RefreshExpiresIn int
}

type AuthUseCase interface {
	Login(ctx context.Context, username, password string) (*domain.User, TokenPair, error)
	// Refresh đổi refresh token còn hiệu lực lấy cặp token mới. Refresh token cũ
	// bị thu hồi ngay (xoay vòng) — dùng lại lần hai sẽ thất bại.
	Refresh(ctx context.Context, refreshToken string) (*domain.User, TokenPair, error)
	// Logout thu hồi session VÀ vô hiệu access token đang dùng.
	// refreshToken rỗng nghĩa là thu hồi MỌI session của user.
	Logout(ctx context.Context, userID uuid.UUID, refreshToken, accessToken string) error
	GetMe(ctx context.Context, userID string) (*domain.User, error)
}

type authUseCase struct {
	userRepo    domain.UserRepository
	sessionRepo domain.SessionRepository
	jwtManager  *jwt.Manager
	accessTTL   time.Duration
	refreshTTL  time.Duration
	// revoked lưu access token đã logout. Có thể nil (chưa bật Redis) —
	// khi đó logout chỉ thu hồi session, access token sống tới lúc hết hạn.
	revoked port.Cache
}

// NewAuthUseCase tạo usecase. accessTTL/refreshTTL lấy từ config, không hardcode
// để mọi mốc hết hạn (JWT, cookie, session trong DB) cùng một nguồn sự thật.
func NewAuthUseCase(
	ur domain.UserRepository,
	sr domain.SessionRepository,
	jm *jwt.Manager,
	accessTTL, refreshTTL time.Duration,
	revoked port.Cache,
) AuthUseCase {
	return &authUseCase{
		userRepo:    ur,
		sessionRepo: sr,
		jwtManager:  jm,
		accessTTL:   accessTTL,
		refreshTTL:  refreshTTL,
		revoked:     revoked,
	}
}

func (u *authUseCase) Login(ctx context.Context, username, password string) (*domain.User, TokenPair, error) {
	user, err := u.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, TokenPair{}, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, TokenPair{}, ErrInvalidCredentials
	}

	pair, err := u.issue(ctx, user)
	if err != nil {
		return nil, TokenPair{}, err
	}

	// Đảm bảo không trả về thông tin nhạy cảm.
	user.PasswordHash = ""
	return user, pair, nil
}

func (u *authUseCase) Refresh(ctx context.Context, refreshToken string) (*domain.User, TokenPair, error) {
	if refreshToken == "" {
		return nil, TokenPair{}, ErrInvalidRefresh
	}

	session, err := u.sessionRepo.FindByToken(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return nil, TokenPair{}, ErrInvalidRefresh
		}
		return nil, TokenPair{}, ErrInternalError
	}

	// Hết hạn thì dọn luôn bản ghi rác rồi mới báo lỗi.
	if !session.ExpiresAt.After(time.Now()) {
		_ = u.sessionRepo.Delete(ctx, refreshToken)
		return nil, TokenPair{}, ErrInvalidRefresh
	}

	user, err := u.userRepo.FindByID(ctx, session.UserID.String())
	if err != nil {
		// Session còn nhưng user đã bị xóa — thu hồi để không dùng lại được.
		_ = u.sessionRepo.Delete(ctx, refreshToken)
		return nil, TokenPair{}, ErrInvalidRefresh
	}

	pair, err := u.issue(ctx, user)
	if err != nil {
		return nil, TokenPair{}, err
	}

	// Xoay vòng: thu hồi token cũ SAU KHI đã cấp thành công token mới, để nếu
	// bước cấp lỗi thì client vẫn còn token cũ dùng được.
	_ = u.sessionRepo.Delete(ctx, refreshToken)

	user.PasswordHash = ""
	return user, pair, nil
}

func (u *authUseCase) Logout(ctx context.Context, userID uuid.UUID, refreshToken, accessToken string) error {
	u.revokeAccessToken(ctx, accessToken)

	if refreshToken != "" {
		return u.sessionRepo.Delete(ctx, refreshToken)
	}
	return u.sessionRepo.DeleteByUserID(ctx, userID)
}

// revokeAccessToken ghi token vào danh sách chặn, hết hạn cùng lúc với token.
//
// Lỗi ở đây KHÔNG làm hỏng logout: session đã bị xóa nên refresh token mất
// hiệu lực, phần tệ nhất còn lại chỉ là access token sống nốt thời gian ngắn
// còn lại của nó.
func (u *authUseCase) revokeAccessToken(ctx context.Context, accessToken string) {
	if u.revoked == nil || accessToken == "" {
		return
	}
	claims, err := u.jwtManager.Verify(accessToken)
	if err != nil {
		return // token hỏng hoặc đã hết hạn thì không cần chặn nữa
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		return
	}
	if err := u.revoked.Set(ctx, tokenrevoke.Key(accessToken), "1", ttl); err != nil {
		logger.FromContext(ctx).Error("không ghi được access token vào danh sách thu hồi",
			zap.Error(err))
	}
}

func (u *authUseCase) GetMe(ctx context.Context, userID string) (*domain.User, error) {
	user, err := u.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.PasswordHash = ""
	return user, nil
}

// issue cấp cặp token mới và lưu session tương ứng.
func (u *authUseCase) issue(ctx context.Context, user *domain.User) (TokenPair, error) {
	var roles []string
	if user.Roles != "" {
		_ = json.Unmarshal([]byte(user.Roles), &roles)
	}

	now := time.Now()
	accessToken, err := u.jwtManager.Sign(user.ID.String(), user.Username, roles, now)
	if err != nil {
		return TokenPair{}, ErrInternalError
	}

	refreshToken, err := newRefreshToken()
	if err != nil {
		return TokenPair{}, ErrInternalError
	}

	session := &domain.Session{
		ID:        uuid.New(),
		UserID:    user.ID,
		Token:     refreshToken,
		ExpiresAt: now.Add(u.refreshTTL),
		CreatedAt: now,
	}
	if err := u.sessionRepo.Create(ctx, session); err != nil {
		return TokenPair{}, ErrInternalError
	}

	return TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        int(u.accessTTL.Seconds()),
		RefreshExpiresIn: int(u.refreshTTL.Seconds()),
	}, nil
}

// newRefreshToken sinh chuỗi ngẫu nhiên an toàn mật mã, an toàn khi đặt trong URL.
func newRefreshToken() (string, error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("sinh refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
