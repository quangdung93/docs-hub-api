package response

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
)

// TestEnvelope là GOLDEN TEST chống trôi khỏi chuẩn ISC (templates/03).
// So khớp JSON sinh ra với đúng 4 ví dụ trong tài liệu chuẩn.
//
// Nếu ai đó lỡ đổi tên field (error -> errors, thêm message cấp gốc, ...),
// test này đỏ ngay lập tức.
func TestEnvelope(t *testing.T) {
	// Meta cố định để so sánh xác định (không phụ thuộc thời gian thực).
	fixedMeta := Meta{
		RequestID: "req-abc-123",
		TraceID:   "trace-xyz-789",
		Timestamp: "2025-06-16T09:00:00Z",
	}

	tests := []struct {
		name string
		env  Envelope
		want string
	}{
		{
			name: "thành công có data",
			env:  newSuccess(map[string]any{"user_id": 123, "username": "john.doe"}, fixedMeta),
			want: `{
				"success": true,
				"data": {"user_id": 123, "username": "john.doe"},
				"error": null,
				"meta": {
					"request_id": "req-abc-123",
					"trace_id": "trace-xyz-789",
					"timestamp": "2025-06-16T09:00:00Z"
				}
			}`,
		},
		{
			name: "lỗi nghiệp vụ",
			env: newError(&ErrorBody{
				Code:      "INVALID_OTP",
				Message:   "Mã OTP không hợp lệ hoặc đã hết hạn",
				Details:   map[string]any{"field": "sms_otp"},
				Retryable: false,
			}, fixedMeta),
			want: `{
				"success": false,
				"data": null,
				"error": {
					"code": "INVALID_OTP",
					"message": "Mã OTP không hợp lệ hoặc đã hết hạn",
					"details": {"field": "sms_otp"},
					"retryable": false
				},
				"meta": {
					"request_id": "req-abc-123",
					"trace_id": "trace-xyz-789",
					"timestamp": "2025-06-16T09:00:00Z"
				}
			}`,
		},
		{
			name: "lỗi kỹ thuật",
			env: newError(&ErrorBody{
				Code:      "SYS_500",
				Message:   "Internal server error. Please try again later.",
				Details:   map[string]any{"field": "Can not connect database."},
				Retryable: true,
			}, fixedMeta),
			want: `{
				"success": false,
				"data": null,
				"error": {
					"code": "SYS_500",
					"message": "Internal server error. Please try again later.",
					"details": {"field": "Can not connect database."},
					"retryable": true
				},
				"meta": {
					"request_id": "req-abc-123",
					"trace_id": "trace-xyz-789",
					"timestamp": "2025-06-16T09:00:00Z"
				}
			}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.env)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(got))
		})
	}
}

// TestEnvelope_Paginated kiểm tra nhánh có phân trang (đúng 6 field meta.pagination).
func TestEnvelope_Paginated(t *testing.T) {
	meta := Meta{
		RequestID: "req-xyz-111",
		TraceID:   "trace-abc-222",
		Timestamp: "2025-06-16T09:03:00Z",
		Pagination: &pagination.Meta{
			Page: 1, Limit: 10, TotalItems: 42, TotalPages: 5, HasNext: true, HasPrev: false,
		},
	}
	env := newSuccess([]any{}, meta)

	got, err := json.Marshal(env)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"success": true,
		"data": [],
		"error": null,
		"meta": {
			"request_id": "req-xyz-111",
			"trace_id": "trace-abc-222",
			"timestamp": "2025-06-16T09:03:00Z",
			"pagination": {
				"page": 1,
				"limit": 10,
				"total_items": 42,
				"total_pages": 5,
				"has_next": true,
				"has_prev": false
			}
		}
	}`, string(got))
}

// TestEnvelope_NoTopLevelMessageField khẳng định KHÔNG có "message" ở cấp gốc
// và KHÔNG có "errors" (số nhiều) — hai sai lệch thường gặp so với chuẩn ISC.
func TestEnvelope_NoTopLevelMessageField(t *testing.T) {
	env := newSuccess(nil, Meta{Timestamp: "2025-01-01T00:00:00Z"})
	got, err := json.Marshal(env)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(got, &raw))

	_, hasMessage := raw["message"]
	require.False(t, hasMessage, `envelope KHÔNG được có field "message" ở cấp gốc`)
	_, hasErrors := raw["errors"]
	require.False(t, hasErrors, `phải là "error" (số ít), không phải "errors"`)
	_, hasError := raw["error"]
	require.True(t, hasError, `phải có field "error" (kể cả khi null)`)
}
