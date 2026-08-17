package ingestion

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateVectors(t *testing.T) {
	tests := []struct {
		name       string
		vectors    [][]float32
		count, dim int
		wantErr    bool
	}{
		{"hợp lệ", [][]float32{{1, 2}, {3, 4}}, 2, 2, false},
		{"tự nhận dimension", [][]float32{{1, 2}, {3, 4}}, 2, 0, false},
		{"thiếu vector", [][]float32{{1, 2}}, 2, 2, true},
		{"sai dimension", [][]float32{{1, 2}, {3}}, 2, 2, true},
		{"vector rỗng", [][]float32{{}}, 1, 0, true},
		{"NaN", [][]float32{{float32(math.NaN())}}, 1, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVectors(tt.vectors, tt.count, tt.dim)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
