package domain

import (
	"context"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
)

// UserRepository là PORT (interface) mà usecase phụ thuộc. Implementation cụ thể
// bằng GORM nằm ở tầng repository. Đây là ranh giới đảo ngược phụ thuộc: domain
// định nghĩa hợp đồng, hạ tầng tuân theo.
//
// Quy ước lỗi trả về (để usecase quyết định ngữ nghĩa nghiệp vụ):
//   - ErrNotFound       : không có bản ghi khớp.
//   - ErrDuplicate      : vi phạm ràng buộc duy nhất (email).
//   - ErrNoRowsAffected : update/delete không tác động dòng nào (dùng cho optimistic lock).
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	// Update cập nhật theo optimistic lock (khớp cả version). Không khớp -> ErrNoRowsAffected.
	Update(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	// ExistsByEmail kiểm tra email đã dùng chưa; excludeID để bỏ qua chính user đang xét.
	ExistsByEmail(ctx context.Context, email string, excludeID *uuid.UUID) (bool, error)
	List(ctx context.Context, f Filter, page pagination.Query) ([]User, int64, error)
	// SoftDelete xóa mềm theo optimistic lock. Không khớp version -> ErrNoRowsAffected.
	SoftDelete(ctx context.Context, id uuid.UUID, version int) error
}

// Lỗi hợp đồng của repository (độc lập với chi tiết driver/ORM).
var (
	ErrNotFound       = newRepoError("không tìm thấy bản ghi")
	ErrDuplicate      = newRepoError("vi phạm ràng buộc duy nhất")
	ErrNoRowsAffected = newRepoError("không có dòng nào bị tác động")
)

type repoError struct{ msg string }

func newRepoError(msg string) *repoError { return &repoError{msg: msg} }
func (e *repoError) Error() string       { return "user repository: " + e.msg }
