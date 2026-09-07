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
