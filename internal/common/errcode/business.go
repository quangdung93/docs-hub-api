// Package errcode chứa toàn bộ mã lỗi chuẩn ISC (templates/04) dưới dạng hằng số,
// cùng bảng ánh xạ mã kỹ thuật -> HTTP status.
//
// Nguyên tắc: KHÔNG hardcode chuỗi mã lỗi rải rác trong code. Mọi nơi tham chiếu
// hằng số ở đây để đồng nhất và dễ trace/monitor.
package errcode

// Mã lỗi NGHIỆP VỤ — trả về HTTP 200 kèm success=false (do service chủ động trả).
// Đây là các mã trong bảng templates/04.
const (
	DuplicateEmail     = "DUPLICATE_EMAIL"     // Email đã tồn tại trong hệ thống
	UserLocked         = "USER_LOCKED"         // Tài khoản người dùng bị khóa
	InvalidPass        = "INVALID_PASS"        // Mật khẩu không đúng định dạng
	InvalidOTP         = "INVALID_OTP"         // Mã OTP không hợp lệ hoặc đã hết hạn
	MissingCaptcha     = "MISSING_CAPTCHA"     // Thiếu CAPTCHA trong request
	UnauthorizedDevice = "UNAUTHORIZED_DEVICE" // Thiết bị không được phép truy cập
	InviteExpired      = "INVITE_EXPIRED"      // Link mời đã hết hạn hoặc không tồn tại
	InvalidProfile     = "INVALID_PROFILE"     // Thông tin hồ sơ không hợp lệ
	NotifyFailed       = "NOTIFY_FAILED"       // Gửi thông báo thất bại
	SessionConflict    = "SESSION_CONFLICT"    // Đăng nhập đồng thời gây xung đột session
	FileTooLarge       = "FILE_TOO_LARGE"      // File upload vượt giới hạn
	ImageInvalid       = "IMAGE_INVALID"       // Định dạng ảnh không hỗ trợ
	MFARequired        = "MFA_REQUIRED"        // Cần xác thực đa yếu tố (MFA)

	// ConflictVersion — mã nghiệp vụ BỔ SUNG (không có trong templates/04 gốc),
	// dùng cho optimistic lock. Cần TL duyệt (xem ADR-0003). Nhóm BUSINESS_RULE_*.
	// Fallback nếu bị từ chối: dùng SessionConflict.
	ConflictVersion = "CONFLICT_VERSION" // Dữ liệu đã bị thay đổi bởi request khác

	// AlreadyMember — mã nghiệp vụ BỔ SUNG cho module project: user đã là thành
	// viên (hoặc đang có lời mời chờ xác nhận) của dự án.
	AlreadyMember = "ALREADY_MEMBER" // Người dùng đã là thành viên hoặc đang chờ xác nhận
	// InviteNotPending — lời mời không ở trạng thái 'pending' nên không thể accept.
	InviteNotPending = "INVITE_NOT_PENDING" // Lời mời không ở trạng thái chờ xác nhận
	// CannotModifyOwner — không được đổi role/gỡ chủ dự án qua API quản lý thành viên.
	CannotModifyOwner = "CANNOT_MODIFY_OWNER" // Không thể đổi vai trò hoặc gỡ chủ dự án
	// ConfirmNameMismatch — xóa dự án yêu cầu gửi đúng tên dự án để xác nhận.
	ConfirmNameMismatch = "CONFIRM_NAME_MISMATCH" // Tên xác nhận không khớp tên dự án
)
