// Package apperr định nghĩa hai loại lỗi ứng dụng theo chuẩn ISC:
//
//   - BusinessError : lỗi nghiệp vụ, LUÔN trả HTTP 200 + success=false.
//     Do service chủ động trả về, không panic.
//   - TechnicalError: lỗi kỹ thuật, trả HTTP 4xx/5xx qua middleware.
//
// Hai type cố tình KHÔNG dùng chung interface có HTTPStatus(): một lỗi nghiệp vụ
// không hề có HTTP status, và việc gán status cho nó chính là nguồn gốc của lỗi
// phổ biến "trả 5xx cho lỗi nghiệp vụ" mà thiết kế này muốn triệt tiêu.
package apperr

import "fmt"

// BusinessError là lỗi nghiệp vụ. Middleware sẽ render nó thành HTTP 200.
type BusinessError struct {
	Code      string // mã từ errcode/business.go
	Message   string // thông điệp tiếng Việt, hiển thị được cho người dùng
	Details   any    // chi tiết bổ sung (object/array), có thể nil
	Retryable bool   // client có thể thử lại không
	cause     error  // lỗi gốc (để log/trace), KHÔNG lộ ra client
}

// NewBusiness tạo một BusinessError. Thường được khai báo dưới dạng sentinel
// ở tầng domain (ví dụ domain.ErrDuplicateEmail).
func NewBusiness(code, message string, retryable bool) *BusinessError {
	return &BusinessError{Code: code, Message: message, Retryable: retryable}
}

func (e *BusinessError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("business error %s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("business error %s: %s", e.Code, e.Message)
}

// Unwrap cho phép errors.Is/As xuyên tới lỗi gốc.
func (e *BusinessError) Unwrap() error { return e.cause }

// WithDetails trả về BẢN SAO kèm details — không mutate sentinel gốc.
// Nhờ vậy có thể an toàn dùng `return domain.ErrX.WithDetails(...)`.
func (e *BusinessError) WithDetails(details any) *BusinessError {
	clone := *e
	clone.Details = details
	return &clone
}

// WithCause trả về bản sao kèm lỗi gốc (để phục vụ log/trace).
func (e *BusinessError) WithCause(cause error) *BusinessError {
	clone := *e
	clone.cause = cause
	return &clone
}
