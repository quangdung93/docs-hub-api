package localai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

const maxChatResponseBytes = 1 << 20

// ChatClient gọi API chat completions tương thích OpenAI của LocalAI.
type ChatClient struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewChatClient(baseURL, model string, timeout time.Duration) *ChatClient {
	return &ChatClient{
		baseURL: strings.TrimRight(baseURL, "/"), model: strings.TrimSpace(model),
		client: &http.Client{Timeout: timeout},
	}
}

func (c *ChatClient) Complete(ctx context.Context, input port.ChatCompletionRequest) (port.ChatCompletionResult, error) {
	if c.model == "" {
		return port.ChatCompletionResult{}, fmt.Errorf("thiếu cấu hình local_ai.chat_model")
	}
	body, err := json.Marshal(map[string]any{
		"model": c.model, "temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": input.SystemPrompt},
			{"role": "user", "content": input.UserPrompt},
		},
	})
	if err != nil {
		return port.ChatCompletionResult{}, fmt.Errorf("encode LocalAI chat request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return port.ChatCompletionResult{}, fmt.Errorf("tạo LocalAI chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return port.ChatCompletionResult{}, fmt.Errorf("gọi LocalAI chat: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return port.ChatCompletionResult{}, fmt.Errorf("LocalAI chat trả HTTP %d", res.StatusCode)
	}
	reader := io.LimitReader(res.Body, maxChatResponseBytes+1)
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.NewDecoder(reader).Decode(&out); err != nil {
		return port.ChatCompletionResult{}, fmt.Errorf("decode LocalAI chat: %w", err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return port.ChatCompletionResult{}, fmt.Errorf("LocalAI chat không trả nội dung")
	}
	return port.ChatCompletionResult{Content: strings.TrimSpace(out.Choices[0].Message.Content), Model: out.Model}, nil
}
