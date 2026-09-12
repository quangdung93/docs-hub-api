// Package errcode chứa toàn bộ mã lỗi chuẩn ISC (templates/04) dưới dạng hằng số,
// cùng bảng ánh xạ mã kỹ thuật -> HTTP status.
//
// Nguyên tắc: KHÔNG hardcode chuỗi mã lỗi rải rác trong code. Mọi nơi tham chiếu
// hằng số ở đây để đồng nhất và dễ trace/monitor.
package errcode

// Mã lỗi NGHIỆP VỤ — trả về HTTP 200 kèm success=false (do service chủ động trả).
// Đây là các mã trong bảng templates/04.
const (
	DuplicateEmail       = "DUPLICATE_EMAIL"        // Email đã tồn tại trong hệ thống
	UserLocked           = "USER_LOCKED"            // Tài khoản người dùng bị khóa
	InvalidPass          = "INVALID_PASS"           // Mật khẩu không đúng định dạng
	InvalidOTP           = "INVALID_OTP"            // Mã OTP không hợp lệ hoặc đã hết hạn
	MissingCaptcha       = "MISSING_CAPTCHA"        // Thiếu CAPTCHA trong request
	UnauthorizedDevice   = "UNAUTHORIZED_DEVICE"    // Thiết bị không được phép truy cập
	InviteExpired        = "INVITE_EXPIRED"         // Link mời đã hết hạn hoặc không tồn tại
	InvalidProfile       = "INVALID_PROFILE"        // Thông tin hồ sơ không hợp lệ
	NotifyFailed         = "NOTIFY_FAILED"          // Gửi thông báo thất bại
	SessionConflict      = "SESSION_CONFLICT"       // Đăng nhập đồng thời gây xung đột session
	FileTooLarge         = "FILE_TOO_LARGE"         // File upload vượt giới hạn
	ImageInvalid         = "IMAGE_INVALID"          // Định dạng ảnh không hỗ trợ
	MFARequired          = "MFA_REQUIRED"           // Cần xác thực đa yếu tố (MFA)
	UploadInvalid        = "UPLOAD_INVALID"         // Phiên upload không hợp lệ hoặc hết hạn
	DocumentRetryInvalid = "DOCUMENT_RETRY_INVALID" // Revision không thể retry
	ProjectCodeExists    = "PROJECT_CODE_EXISTS"    // Mã project đã tồn tại
	VersionLabelExists   = "VERSION_LABEL_EXISTS"   // Label version đã tồn tại trong project

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
	// AvatarNotUploaded — xác nhận upload ảnh đại diện dự án nhưng ảnh chưa
	// thực sự tồn tại trong storage.
	AvatarNotUploaded = "AVATAR_NOT_UPLOADED" // Ảnh đại diện chưa được tải lên storage
	// DuplicateContent — mã nghiệp vụ BỔ SUNG cho module document: file vừa nạp
	// có nội dung trùng một revision đang hoạt động trong cùng scope. Cùng nhóm
	// với DuplicateEmail của templates/04. Cần TL duyệt (xem ADR-0007).
	//
	// Fallback nếu bị từ chối: UploadInvalid — đã có sẵn trong templates/04 và
	// cùng luồng upload. Cố ý KHÔNG chọn ConflictVersion: mã đó cũng đang chờ
	// duyệt (ADR-0003), lấy nó làm chỗ lui thì không giải quyết được gì.
	DuplicateContent = "DUPLICATE_CONTENT" // Nội dung tài liệu đã tồn tại trong phạm vi này

	// URDNotConfirmed — mã nghiệp vụ BỔ SUNG cho module urd: chưa xác nhận
	// tài liệu là URD (documents.doc_type rỗng) nên chưa thể phân tích edge
	// case. Cần TL duyệt (xem ADR-0008).
	URDNotConfirmed = "URD_NOT_CONFIRMED" // Tài liệu chưa được xác nhận là URD
	// URDRevisionNotReady — mã nghiệp vụ BỔ SUNG cho module urd: revision mới
	// nhất chưa ingest xong (status khác "ready") nên chưa có canonical text
	// để phân tích. Cần TL duyệt (xem ADR-0009).
	URDRevisionNotReady = "URD_REVISION_NOT_READY" // Phiên bản tài liệu chưa sẵn sàng để phân tích
	// URDAnalysisActive — mã nghiệp vụ BỔ SUNG cho module urd: tài liệu đang
	// có 1 phân tích edge case chưa hoàn tất (status analyzing/awaiting_input);
	// chặn phân tích lại để tránh mất dữ liệu người dùng đang nhập dở. Cần TL
	// duyệt (xem ADR-0010).
	URDAnalysisActive = "URD_ANALYSIS_ACTIVE" // Tài liệu đang có phân tích edge case chưa hoàn tất
	// URDCaseUnresolved — mã nghiệp vụ BỔ SUNG cho module urd: còn edge case
	// chưa nhập hướng giải quyết nên chưa thể tạo phiên bản URD mới. Cần TL
	// duyệt (xem ADR-0011).
	URDCaseUnresolved = "URD_CASE_UNRESOLVED" // Còn edge case chưa nhập hướng giải quyết
)
