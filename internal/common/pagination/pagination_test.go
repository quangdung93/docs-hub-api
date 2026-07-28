package pagination_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/quangdung393/docs-hub-api/internal/common/pagination"
)

func TestNormalize_AppliesDefaultsAndCaps(t *testing.T) {
	got := pagination.Query{Page: 0, Limit: 0, Order: "weird"}.Normalize()
	require.Equal(t, 1, got.Page)
	require.Equal(t, 20, got.Limit)
	require.Equal(t, "desc", got.Order)

	capped := pagination.Query{Page: 3, Limit: 9999, Order: "asc"}.Normalize()
	require.Equal(t, 100, capped.Limit, "limit phải bị chặn ở 100")
	require.Equal(t, "asc", capped.Order)
	require.Equal(t, 200, capped.Offset())
}

// TestNewMeta_Boundaries kiểm tra các mốc biên: total=0, total=limit, có trang sau.
func TestNewMeta_Boundaries(t *testing.T) {
	t.Run("total=0", func(t *testing.T) {
		m := pagination.NewMeta(1, 20, 0)
		require.Equal(t, int64(0), m.TotalPages)
		require.False(t, m.HasNext)
		require.False(t, m.HasPrev)
	})

	t.Run("total=limit (vừa đúng 1 trang)", func(t *testing.T) {
		m := pagination.NewMeta(1, 20, 20)
		require.Equal(t, int64(1), m.TotalPages)
		require.False(t, m.HasNext)
		require.False(t, m.HasPrev)
	})

	t.Run("có trang sau", func(t *testing.T) {
		m := pagination.NewMeta(1, 10, 42)
		require.Equal(t, int64(5), m.TotalPages)
		require.True(t, m.HasNext)
		require.False(t, m.HasPrev)
	})

	t.Run("trang giữa", func(t *testing.T) {
		m := pagination.NewMeta(2, 10, 42)
		require.True(t, m.HasNext)
		require.True(t, m.HasPrev)
	})

	t.Run("trang cuối", func(t *testing.T) {
		m := pagination.NewMeta(5, 10, 42)
		require.False(t, m.HasNext)
		require.True(t, m.HasPrev)
	})
}
