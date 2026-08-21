package ingestion

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunkText_GiuLineMappingVaOverlap(t *testing.T) {
	chunks := ChunkText("a\nb\nc\nd\ne", 3, 1)
	require.Len(t, chunks, 2)
	require.Equal(t, 1, chunks[0].LineStart)
	require.Equal(t, 3, chunks[0].LineEnd)
	require.Equal(t, "c\nd\ne", chunks[1].Content)
	require.Equal(t, 3, chunks[1].LineStart)
}

func TestChunkText_GiuNguyenNoiDungVaChuanHoaNewline(t *testing.T) {
	chunks := ChunkText("\r\n  dòng một  \r\n\r\n# Phần hai\r\nnội dung\r\n", 3, 0)
	require.Len(t, chunks, 2)
	require.Equal(t, "  dòng một  ", chunks[0].Content)
	require.Equal(t, 2, chunks[0].LineStart)
	require.Equal(t, "# Phần hai\nnội dung", chunks[1].Content)
	require.Equal(t, 4, chunks[1].LineStart)
}

func TestChunkText_UuTienRanhGioiHeading(t *testing.T) {
	chunks := ChunkText("a\nb\nc\n# Mục mới\nd\ne", 5, 0)
	require.Len(t, chunks, 2)
	require.Equal(t, "a\nb\nc", chunks[0].Content)
	require.Equal(t, "# Mục mới\nd\ne", chunks[1].Content)
}
