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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isNoActionResolution(tc.resolution))
		})
	}
}
