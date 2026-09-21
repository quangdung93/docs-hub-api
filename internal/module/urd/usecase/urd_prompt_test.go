package usecase

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// AI phải trả kèm target_heading (mục nên bổ sung nội dung vào) — thiếu nó
// thì mọi case đều rơi về phụ lục cuối tài liệu thay vì chèn thẳng vào mục.
func TestParseEdgeCases_LayCaMoTaVaTieuDeMuc(t *testing.T) {
	raw := `{"cases":[
		{"description":"Dang nhap sai qua so lan cho phep","target_heading":"Tiêu chí chấp nhận"},
		{"description":"Mat khau qua ngan","target_heading":""}
	]}`

	cases, err := parseEdgeCases(raw)

	require.NoError(t, err)
	require.Len(t, cases, 2)
	require.Equal(t, "Dang nhap sai qua so lan cho phep", cases[0].description)
	require.Equal(t, "Tiêu chí chấp nhận", cases[0].targetHeading)
	require.Empty(t, cases[1].targetHeading, "AI không xác định được mục thì để rỗng")
}

// Phản hồi cũ (chỉ có description) vẫn phải đọc được — RAGFlow không phải
// lúc nào cũng theo đúng schema mới, và mất cả phân tích chỉ vì thiếu 1
// trường phụ là cái giá quá đắt.
func TestParseEdgeCases_ThieuTieuDeMucVanDocDuoc(t *testing.T) {
	cases, err := parseEdgeCases(`{"cases":[{"description":"Case khong co target_heading"}]}`)

	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Equal(t, "Case khong co target_heading", cases[0].description)
	require.Empty(t, cases[0].targetHeading)
}

func TestParseEdgeCases_BoQuaCaseThieuMoTa(t *testing.T) {
	cases, err := parseEdgeCases(`{"cases":[{"description":"","target_heading":"Muc X"},{"description":"Co mo ta"}]}`)

	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Equal(t, "Co mo ta", cases[0].description)
}

// Prompt phải nêu rõ yêu cầu copy nguyên văn tiêu đề: AI tự nghĩ ra tiêu đề
// mới thì docxmerge không khớp được mục nào, tính năng chèn thẳng vô hiệu.
func TestUrdPrompt_YeuCauCopyNguyenVanTieuDe(t *testing.T) {
	prompt := urdPrompt("Du an X", "# Tiêu chí chấp nhận\nAC-01 ...")

	require.Contains(t, prompt, "target_heading")
	require.Contains(t, prompt, "COPY NGUYÊN VĂN")
	require.Contains(t, prompt, "Du an X")
}
