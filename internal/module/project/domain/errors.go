package domain

import (
	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
)

// Lỗi NGHIỆP VỤ của module project (trả HTTP 200 + success=false).
var (
	// ErrAlreadyMember — user đã là thành viên (hoặc đang có lời mời chờ xác
	// nhận) khi mời thêm lần nữa.
	ErrAlreadyMember = apperr.NewBusiness(
		errcode.AlreadyMember, "Người dùng đã là thành viên hoặc đang chờ xác nhận lời mời", false)

	// ErrInviteNotPending — accept một lời mời không còn ở trạng thái pending.
	ErrInviteNotPending = apperr.NewBusiness(
		errcode.InviteNotPending, "Lời mời không ở trạng thái chờ xác nhận", false)

	// ErrCannotModifyOwner — đổi role/gỡ chính owner qua API quản lý thành viên.
	// Owner luôn phải có đúng 1 hàng role=owner trong project_members (khớp
	// projects.owner_id); nếu cho phép đổi/gỡ, owner sẽ tự khóa quyền quản lý
	// chính dự án của mình (RequireProjectRole tra role qua project_members,
	// không qua projects.owner_id).
	ErrCannotModifyOwner = apperr.NewBusiness(
		errcode.CannotModifyOwner, "Không thể đổi vai trò hoặc gỡ chủ dự án qua API này", false)

	// ErrConfirmNameMismatch — xóa dự án yêu cầu xác nhận đúng tên dự án (BR
	// SRS: "xóa dự án không thể hoàn tác, cần xác nhận hai bước"). Tên gửi lên
	// không khớp tên hiện tại của dự án -> chặn, không xóa.
	ErrConfirmNameMismatch = apperr.NewBusiness(
		errcode.ConfirmNameMismatch, "Tên xác nhận không khớp tên dự án", true)
)

// ErrProjectNotFound trả về lỗi KỸ THUẬT 404 (PRJ_404) — dùng khi không tìm
// thấy dự án.
func ErrProjectNotFound() *apperr.TechnicalError {
	return apperr.NotFound(errcode.ProjectNotFound, "Không tìm thấy dự án")
}

// ErrMemberNotFound trả về lỗi KỸ THUẬT 404 (MBR_404) — dùng khi không tìm
// thấy thành viên trong dự án.
func ErrMemberNotFound() *apperr.TechnicalError {
	return apperr.NotFound(errcode.MemberNotFound, "Không tìm thấy thành viên trong dự án")
}
