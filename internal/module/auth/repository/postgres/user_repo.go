package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/module/auth/domain"
)

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

// FindByUsername tìm user đang HOẠT ĐỘNG theo email.
//
// Phải tự lọc deleted_at: truy vấn dùng Table("users") với tên bảng dạng
// chuỗi nên GORM KHÔNG áp scope soft-delete như khi dùng model. Thiếu điều
// kiện này thì user đã xóa vẫn đăng nhập được — xóa tài khoản thành vô tác dụng.
func (r *userRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Table("users").
		Where("email = ? AND deleted_at IS NULL", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("không tìm thấy user")
		}
		return nil, err
	}
	return &user, nil
}

// FindByID tìm user đang HOẠT ĐỘNG theo id. Cũng phải tự lọc deleted_at:
// dùng cho /auth/me và cho luồng refresh token.
func (r *userRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Table("users").
		Where("id = ? AND deleted_at IS NULL", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("không tìm thấy user")
		}
		return nil, err
	}
	return &user, nil
}
