package apperr

import (
	"fmt"

	"github.com/quangdung393/docs-hub-api/internal/common/errcode"
)

// TechnicalError là lỗi kỹ thuật. Middleware render thành HTTP 4xx/5xx.
type TechnicalError struct {
	Code       string // mã từ errcode/technical.go
	HTTPStatus int    // suy ra từ errcode.HTTPStatusOf(Code)
	Message    string // thông điệp an toàn để trả client (không lộ chi tiết nội bộ)
	Details    any    // chi tiết bổ sung, có thể nil
	Retryable  bool
	cause      error // lỗi gốc để log, KHÔNG lộ ra client
}

// newTechnical là constructor nội bộ, tự suy HTTP status từ mã.
func newTechnical(code, message string, retryable bool) *TechnicalError {
	return &TechnicalError{
		Code:       code,
		HTTPStatus: errcode.HTTPStatusOf(code),
		Message:    message,
		Retryable:  retryable,
	}
}

func (e *TechnicalError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("technical error %s (%d): %s: %v", e.Code, e.HTTPStatus, e.Message, e.cause)
	}
	return fmt.Sprintf("technical error %s (%d): %s", e.Code, e.HTTPStatus, e.Message)
}

func (e *TechnicalError) Unwrap() error { return e.cause }

// WithDetails trả về bản sao kèm details.
func (e *TechnicalError) WithDetails(details any) *TechnicalError {
	clone := *e
	clone.Details = details
	return &clone
}

// WithCause trả về bản sao kèm lỗi gốc để log/trace.
func (e *TechnicalError) WithCause(cause error) *TechnicalError {
	clone := *e
	clone.cause = cause
	return &clone
}

// --- Constructor tiện dụng cho các tình huống kỹ thuật thường gặp ---

// BadRequest — REQ_400 (400). Dùng cho lỗi bind/validate.
func BadRequest(message string) *TechnicalError {
	return newTechnical(errcode.Req400, message, false)
}

// Unauthorized — AUTH_401 (401).
func Unauthorized(message string) *TechnicalError {
	return newTechnical(errcode.Auth401, message, false)
}

// Forbidden — AUTH_403 (403).
func Forbidden(message string) *TechnicalError {
	return newTechnical(errcode.Auth403, message, false)
}

// NotFound — mã tùy biến (ví dụ USR_404) map sang 404.
func NotFound(code, message string) *TechnicalError {
	return newTechnical(code, message, false)
}

// Timeout — REQ_TIMEOUT (408).
func Timeout(message string) *TechnicalError {
	return newTechnical(errcode.ReqTimeout, message, true)
}

// TooManyRequests — RATE_429 (429).
func TooManyRequests(message string) *TechnicalError {
	return newTechnical(errcode.Rate429, message, true)
}

// Database — DB_500 (500). retryable=true vì thường do lỗi tạm thời.
func Database(message string) *TechnicalError {
	return newTechnical(errcode.DB500, message, true)
}

// DatabaseUnavailable — DB_503 (503).
func DatabaseUnavailable(message string) *TechnicalError {
	return newTechnical(errcode.DB503, message, true)
}

// MessageQueue — MQ_502 (502).
func MessageQueue(message string) *TechnicalError {
	return newTechnical(errcode.MQ502, message, true)
}

// External — EXT_504 (504).
func External(message string) *TechnicalError {
	return newTechnical(errcode.Ext504, message, true)
}

// Internal — SYS_500 (500). Thông điệp trả client luôn generic.
func Internal(message string) *TechnicalError {
	return newTechnical(errcode.Sys500, message, false)
}
