package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id" gorm:"column:id"`
	Username     string    `json:"username" gorm:"column:email"` // Hệ thống dùng Email làm Username đăng nhập
	PasswordHash string    `json:"-" gorm:"column:password_hash"`
	Roles        string    `json:"roles" gorm:"column:roles"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
}

// ErrSessionNotFound cho phép usecase phân biệt "không tìm thấy" với lỗi hạ tầng.
var ErrSessionNotFound = errors.New("không tìm thấy session")

// Session lưu MỘT refresh token còn hiệu lực. Access token không được lưu ở
// đây: nó ngắn hạn và xác thực bằng chữ ký, không cần tra database.
type Session struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (*User, error)
	FindByID(ctx context.Context, id string) (*User, error)
}

type SessionRepository interface {
	Create(ctx context.Context, session *Session) error
	FindByToken(ctx context.Context, token string) (*Session, error)
	Delete(ctx context.Context, token string) error
	// DeleteByUserID thu hồi MỌI session của user — dùng cho "đăng xuất khỏi mọi thiết bị".
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
}
