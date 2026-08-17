// Package deterministic tạo embedding giả, ổn định để chạy ingestion ở local
// mà không cần một dịch vụ AI bên ngoài.
package deterministic

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

const DefaultDimension = 64

// Embedder biến nội dung thành vector ổn định. Vector này chỉ phục vụ phát triển
// local và kiểm thử pipeline, không có khả năng tìm kiếm ngữ nghĩa như model AI.
type Embedder struct {
	dimension int
}

func New(dimension int) *Embedder {
	if dimension < 1 {
		dimension = DefaultDimension
	}
	return &Embedder{dimension: dimension}
}

func (e *Embedder) Embed(ctx context.Context, input []string) ([][]float32, error) {
	vectors := make([][]float32, len(input))
	for i, value := range input {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vectors[i] = e.vector(value)
	}
	return vectors, nil
}

func (e *Embedder) vector(value string) []float32 {
	vector := make([]float32, e.dimension)
	var sumSquares float64
	for offset, round := 0, uint32(0); offset < len(vector); round++ {
		hash := sha256.New()
		_, _ = hash.Write([]byte(value))
		var suffix [4]byte
		binary.BigEndian.PutUint32(suffix[:], round)
		_, _ = hash.Write(suffix[:])
		for _, b := range hash.Sum(nil) {
			if offset == len(vector) {
				break
			}
			component := float32(int(b)-127) / 128
			vector[offset] = component
			sumSquares += float64(component * component)
			offset++
		}
	}

	// Chuẩn hóa để hình dạng giống embedding thực và cosine distance ổn định.
	norm := float32(math.Sqrt(sumSquares))
	if norm > 0 {
		for i := range vector {
			vector[i] /= norm
		}
	}
	return vector
}
