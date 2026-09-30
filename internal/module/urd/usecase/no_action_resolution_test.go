package usecase

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsNoActionResolution(t *testing.T) {
	tests := []struct {
		name       string
		resolution string
		want       bool
	}{
		{"không", "Không", true},
		{"không có dấu câu cuối câu", "Không.", true},
		{"chưa có chức năng này có khoảng trắng thừa", "  Chưa có chức năng này  ", true},
		{"không áp dụng khác hoa/thường", "KHÔNG ÁP DỤNG", true},
		{"n/a", "N/A", true},
		{"hướng giải quyết thật có nội dung", "Validate dinh dang dd/mm/yyyy", false},
		{"câu dài chứa từ không nhưng có nội dung nghiệp vụ", "Không khóa tài khoản trong giai đoạn beta", false},
		{"rỗng", "", false},
		{"bỏ qua", "Bỏ qua", true},
		{"không có hướng giải quyết", "Không có hướng giải quyết", true},
		{"chưa có hướng xử lý kèm lý do", "Chưa có hướng xử lý, chờ PO chốt", true},
		{"bỏ qua kèm lý do", "Bỏ qua do lỗi gửi thông báo", true},
		{"hành vi người dùng", "Hành vi người dùng", true},
		{"ngoài phạm vi kèm lý do", "Ngoài phạm vi (phase 2)", true},
		{"không kèm quyết định nghiệp vụ", "Không, hệ thống khoá tài khoản sau 5 lần", false},
		{"bỏ qua là 1 bước nghiệp vụ", "Bỏ qua bước xác nhận OTP với thiết bị tin cậy", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isNoActionResolution(tc.resolution))
		})
	}
}
