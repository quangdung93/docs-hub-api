package domain

import (
	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
)

// Lỗi NGHIỆP VỤ của module user (trả HTTP 200 + success=false).
// Khai báo dạng sentinel để usecase trả về trực tiếp; dùng WithDetails khi cần.
var (
	// ErrDuplicateEmail — email đã tồn tại khi tạo/đổi email.
	ErrDuplicateEmail = apperr.NewBusiness(
		errcode.DuplicateEmail, "Email đã tồn tại trong hệ thống", false)

	// ErrUserLocked — thao tác không hợp lệ vì tài khoản đang bị khóa.
	ErrUserLocked = apperr.NewBusiness(
		errcode.UserLocked, "Tài khoản người dùng đang bị khóa", false)

	// ErrVersionConflict — cập nhật thất bại vì dữ liệu đã bị thay đổi (optimistic lock).
	ErrVersionConflict = apperr.NewBusiness(
		errcode.ConflictVersion, "Dữ liệu đã bị thay đổi bởi thao tác khác, vui lòng tải lại", true)

	// ErrInvalidStatus — giá trị trạng thái không hợp lệ.
	ErrInvalidStatus = apperr.NewBusiness(
		errcode.InvalidProfile, "Trạng thái người dùng không hợp lệ", false)
)

// ErrUserNotFound trả về lỗi KỸ THUẬT 404 (USR_404) — dùng ở usecase khi không
// tìm thấy user. Là hàm (không phải sentinel) vì apperr có thể kèm cause/details.
func ErrUserNotFound() *apperr.TechnicalError {
	return apperr.NotFound(errcode.UserNotFound, "Không tìm thấy người dùng")
}
