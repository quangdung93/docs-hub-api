package usecase

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
)

func TestNoDataAnswer_NhanDienVanXuoi(t *testing.T) {
	t.Parallel()
	vanXuoi := []string{
		"Sorry! No relevant content was found in the knowledge base!",
		"The answer you are looking for is not found in the dataset!",
		"Xin lỗi, tôi không tìm thấy thông tin phù hợp.",
		"",
		"   ",
	}
	for _, noiDung := range vanXuoi {
		require.True(t, noDataAnswer(noiDung), "phải nhận là văn xuôi: %q", noiDung)
	}

	laJSON := []string{
		`{"items":[]}`,
		"  {\"milestones\":[]}  ",
		"```json\n{\"items\":[]}\n```",
		`{"items":[]} [ID:0]`,
		`[ID:0] {"items":[]}`,
	}
	for _, noiDung := range laJSON {
		require.False(t, noDataAnswer(noiDung), "phải nhận là JSON: %q", noiDung)
	}
}

// Nội dung mở đầu bằng "{" nhưng hỏng thì KHÔNG phải văn xuôi — vẫn đi nhánh
// lỗi kỹ thuật để còn điều tra.
func TestNoDataAnswer_JSONMeoKhongBiNuot(t *testing.T) {
	t.Parallel()
	require.False(t, noDataAnswer(`{"items":[{"title":`))
}

func TestNoDataError_LaLoiNghiepVuVaGiuNguyenVan(t *testing.T) {
	t.Parallel()
	err := noDataError("Sorry! No relevant content was found in the knowledge base!")
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 400, technical.HTTPStatus)
	require.Contains(t, err.Error(), "No relevant content")
}

func TestNoDataError_CatBotNoiDungQuaDai(t *testing.T) {
	t.Parallel()
	dai := make([]rune, noDataSnippetLimit*2)
	for i := range dai {
		dai[i] = 'ă'
	}
	err := noDataError(string(dai))
	require.Contains(t, err.Error(), "…")
	// Không được cắt giữa ký tự nhiều byte.
	require.True(t, len([]rune(err.Error())) < len(dai))
}
