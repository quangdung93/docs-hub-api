package domain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// User là bản đọc gọn của bảng users phục vụ xác thực.
//
// Roles giữ NGUYÊN chuỗi JSON như trong DB (cột roles là VARCHAR). Đừng
// serialize struct này thẳng ra HTTP — dùng RolesList() và DTO ở tầng
// delivery, nếu không client sẽ nhận roles dạng chuỗi và phải parse hai lần.
type User struct {
	ID           uuid.UUID `gorm:"column:id"`
	Username     string    `gorm:"column:email"` // Hệ thống dùng Email làm Username đăng nhập
	FullName     string    `gorm:"column:full_name"`
	PasswordHash string    `gorm:"column:password_hash"`
	Roles        string    `gorm:"column:roles"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

// RolesList giải mã cột roles (JSON) thành mảng. Dữ liệu hỏng thì trả mảng
// rỗng thay vì lỗi — người dùng không có quyền nào còn hơn chặn đăng nhập.
func (u User) RolesList() []string {
	if strings.TrimSpace(u.Roles) == "" {
		return []string{}
	}
	var roles []string
	if err := json.Unmarshal([]byte(u.Roles), &roles); err != nil {
		return []string{}
	}
	return roles
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
