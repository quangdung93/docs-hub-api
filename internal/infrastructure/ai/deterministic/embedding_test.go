package deterministic_test

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/infrastructure/ai/deterministic"
)

func TestEmbedIsDeterministicAndNormalized(t *testing.T) {
	embedder := deterministic.New(16)

	first, err := embedder.Embed(context.Background(), []string{"nội dung", "khác"})
	require.NoError(t, err)
	second, err := embedder.Embed(context.Background(), []string{"nội dung"})
	require.NoError(t, err)

	require.Len(t, first, 2)
	require.Len(t, first[0], 16)
	require.Equal(t, first[0], second[0])
	require.NotEqual(t, first[0], first[1])

	var sumSquares float64
	for _, value := range first[0] {
		sumSquares += float64(value * value)
	}
	require.InDelta(t, 1, math.Sqrt(sumSquares), 0.00001)
}

func TestEmbedUsesDefaultDimension(t *testing.T) {
	vectors, err := deterministic.New(0).Embed(context.Background(), []string{"test"})
	require.NoError(t, err)
	require.Len(t, vectors[0], deterministic.DefaultDimension)
}
