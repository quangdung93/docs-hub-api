// Package jwt ký và xác thực JSON Web Token.
//
// Hỗ trợ HS256 (secret đối xứng). Gói này thuần, không phụ thuộc internal/*,
// để tái sử dụng được ngoài repo.
package jwt

import (
	"errors"
	"fmt"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Lỗi chuẩn hóa để caller phân biệt nguyên nhân.
var (
	ErrInvalidToken = errors.New("token không hợp lệ")
	ErrExpiredToken = errors.New("token đã hết hạn")
)

// Claims là payload của access token.
type Claims struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	jwtlib.RegisteredClaims
}

// Manager ký/verify token với cấu hình cố định.
type Manager struct {
	secret    []byte
	issuer    string
	accessTTL time.Duration
}

// Config là tham số khởi tạo Manager.
type Config struct {
	Secret    string
	Issuer    string
	AccessTTL time.Duration
}

// NewManager tạo Manager. Lỗi nếu secret rỗng.
func NewManager(cfg Config) (*Manager, error) {
	if cfg.Secret == "" {
		return nil, errors.New("jwt: secret rỗng")
	}
	return &Manager{
		secret:    []byte(cfg.Secret),
		issuer:    cfg.Issuer,
		accessTTL: cfg.AccessTTL,
	}, nil
}

// Sign tạo access token cho user với roles cho trước.
func (m *Manager) Sign(userID, email string, roles []string, now time.Time) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		Roles:  roles,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(now.Add(m.accessTTL)),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("ký token thất bại: %w", err)
	}
	return signed, nil
}

// Verify xác thực token và trả về claims. Phân biệt hết hạn vs không hợp lệ.
func (m *Manager) Verify(tokenString string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwtlib.ParseWithClaims(tokenString, claims, func(t *jwtlib.Token) (any, error) {
		if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: thuật toán ký không mong đợi", ErrInvalidToken)
		}
		return m.secret, nil
	}, jwtlib.WithIssuer(m.issuer))

	switch {
	case err == nil:
		return claims, nil
	case errors.Is(err, jwtlib.ErrTokenExpired):
		return nil, ErrExpiredToken
	default:
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
}
