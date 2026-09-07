package usecase

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseUATItems_JSONThuong(t *testing.T) {
	t.Parallel()
	items, err := parseUATItems(`{"items":[{"title":"A","steps":"B","expected":"C","source":"D"}]}`)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "A", items[0].Title)
}

func TestParseUATItems_BocCodeFence(t *testing.T) {
	t.Parallel()
	items, err := parseUATItems("```json\n{\"items\":[{\"title\":\"A\"}]}\n```")
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestParseUATItems_Rong(t *testing.T) {
	t.Parallel()
	items, err := parseUATItems(`{"items":[]}`)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestParseUATItems_JSONHong(t *testing.T) {
	t.Parallel()
	_, err := parseUATItems("không phải json")
	require.Error(t, err)
}

// RAGFlow chèn chú thích trích dẫn dạng [ID:n] khi request bật "reference"
// (dùng chung cho cả module chat) — 2 tình huống thật đã gặp: [ID:n] đứng
// NGAY SAU dấu "}" đóng JSON (làm hỏng parse hoàn toàn) và [ID:n] lọt vào
// GIỮA một giá trị chuỗi (JSON vẫn hợp lệ nhưng nội dung lẫn rác kỹ thuật).
func TestParseUATItems_ChuThichTrichDanSauJSON(t *testing.T) {
	t.Parallel()
	items, err := parseUATItems(`{"items":[{"title":"A","steps":"B","expected":"C","source":"D"}]} [ID:0]`)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "A", items[0].Title)
}

func TestParseUATItems_ChuThichTrichDanTrongChuoi(t *testing.T) {
	t.Parallel()
	items, err := parseUATItems(
		`{"items":[{"title":"A","steps":"Bước 1 [ID:0,3] bước 2","expected":"C [ID: 5 ]","source":"D"}]}`)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Bước 1  bước 2", items[0].Steps)
	require.Equal(t, "C ", items[0].Expected)
}
