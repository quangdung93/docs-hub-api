// Package response định nghĩa envelope phản hồi chuẩn ISC (templates/03) và
// các hàm ghi response. Đây là NƠI DUY NHẤT trong toàn hệ thống được phép ghi
// JSON ra client (golangci-lint forbidigo cấm c.JSON ở mọi nơi khác) — nhờ đó
// mọi response đều đúng một định dạng.
//
// Cấu trúc bắt buộc:
//
//	{
//	  "success": true,
//	  "data": {...} | [...] | null,
//	  "error": null | { "code", "message", "details", "retryable" },
//	  "meta":  { "request_id", "trace_id", "timestamp", "pagination"? }
//	}
//
// Lưu ý: `error` là SỐ ÍT (không phải "errors"), và KHÔNG có "message" ở cấp gốc.
package response

import (
	"github.com/quangdung393/docs-hub-api/internal/common/pagination"
)

// Envelope là cấu trúc phản hồi gốc.
type Envelope struct {
	Success bool       `json:"success"`
	Data    any        `json:"data"`
	Error   *ErrorBody `json:"error"`
	Meta    Meta       `json:"meta"`
}

// ErrorBody là phần lỗi trong envelope. nil => render thành "error": null.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	Retryable bool   `json:"retryable"`
}

// Meta là metadata phục vụ trace/monitoring, luôn có mặt trong mọi response.
type Meta struct {
	RequestID  string           `json:"request_id"`
	TraceID    string           `json:"trace_id"`
	Timestamp  string           `json:"timestamp"` // ISO-8601 UTC
	Pagination *pagination.Meta `json:"pagination,omitempty"`
}

// newSuccess dựng envelope thành công.
func newSuccess(data any, meta Meta) Envelope {
	return Envelope{Success: true, Data: data, Error: nil, Meta: meta}
}

// newError dựng envelope lỗi (data luôn null).
func newError(body *ErrorBody, meta Meta) Envelope {
	return Envelope{Success: false, Data: nil, Error: body, Meta: meta}
}
