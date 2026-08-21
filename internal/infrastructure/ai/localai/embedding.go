// Package localai gọi LocalAI qua OpenAI-compatible HTTP API.
package localai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Embedder struct {
	baseURL, model string
	dimension      int
	client         *http.Client
}

func New(baseURL, model string, dimension int, timeout time.Duration) *Embedder {
	return &Embedder{baseURL: strings.TrimRight(baseURL, "/"), model: model, dimension: dimension, client: &http.Client{Timeout: timeout}}
}
func (e *Embedder) Embed(ctx context.Context, input []string) ([][]float32, error) {
	if e.model == "" {
		return nil, fmt.Errorf("thiếu cấu hình local_ai.embedding_model")
	}
	body, _ := json.Marshal(map[string]any{"model": e.model, "input": input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tạo embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gọi LocalAI embedding: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("LocalAI embedding trả HTTP %d", res.StatusCode)
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err = json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode embedding: %w", err)
	}
	vectors := make([][]float32, len(input))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vectors) {
			return nil, fmt.Errorf("embedding index không hợp lệ")
		}
		if e.dimension > 0 && len(d.Embedding) != e.dimension {
			return nil, fmt.Errorf("embedding dimension %d, cần %d", len(d.Embedding), e.dimension)
		}
		vectors[d.Index] = d.Embedding
	}
	for _, v := range vectors {
		if len(v) == 0 {
			return nil, fmt.Errorf("LocalAI thiếu embedding")
		}
	}
	return vectors, nil
}
