// Package domain chứa entity, quy tắc nghiệp vụ, port repository và lỗi nghiệp vụ
// của module user.
//
// Đây là tầng TRONG CÙNG của Clean Architecture: chỉ phụ thuộc stdlib, uuid và
// common/apperr. TUYỆT ĐỐI không import gin/gorm/redis (được golangci-lint
// depguard bảo vệ). Nhờ vậy business logic độc lập hoàn toàn với framework.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// DefaultAdminEmail là email của tài khoản quản trị mặc định.
//
// Đặt ở đây vì CẢ cmd/seed (lúc tạo) lẫn bootstrap (lúc nạp actor cho môi
// trường local) đều cần đúng một giá trị — để mỗi bên tự viết chuỗi thì đổi
// một bên là chạy local hỏng ngay.
//
// Phải là email HỢP LỆ: POST /users validate `email`, nên giá trị cũ
// "admin@local" (thiếu TLD) tạo ra mâu thuẫn — seed nối thẳng vào DB nên lách
// được validate, còn API thì không.
const DefaultAdminEmail = "admin@docshub.io.vn"

// User là entity người dùng — mô hình nghiệp vụ thuần, KHÔNG có tag gorm.
// Việc ánh xạ sang bảng DB nằm ở tầng repository (userModel + mapper).
type User struct {
	ID           uuid.UUID
	Email        string
	FullName     string
	PasswordHash string
	Status       Status
	Roles        []string
	// Version phục vụ optimistic lock. Đây là khái niệm NGHIỆP VỤ về xung đột
	// đồng thời (client gửi lại khi update), không phải chi tiết ORM.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewUser tạo user mới ở trạng thái Active với version khởi đầu = 1.
func NewUser(email, fullName, passwordHash string, roles []string) *User {
	return &User{
		ID:           uuid.New(),
		Email:        email,
		FullName:     fullName,
		PasswordHash: passwordHash,
		Status:       StatusActive,
		Roles:        roles,
		Version:      1,
	}
}
