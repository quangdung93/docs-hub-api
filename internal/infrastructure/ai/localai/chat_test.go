package localai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

func TestChatClient_GoiOpenAICompatibleEndpoint(t *testing.T) {
	t.Parallel()
	client := NewChatClient("http://localai.test", "qwen-test", time.Second)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(bytes.NewBufferString(
				`{"model":"qwen-test","choices":[{"message":{"content":"Trả lời [S1]."}}]}`)),
		}, nil
	})

	result, err := client.Complete(context.Background(), port.ChatCompletionRequest{
		SystemPrompt: "system", UserPrompt: "question",
	})

	require.NoError(t, err)
	require.Equal(t, "Trả lời [S1].", result.Content)
	require.Equal(t, "qwen-test", result.Model)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestChatClient_TuChoiKhiThieuModel(t *testing.T) {
	t.Parallel()
	client := NewChatClient("http://127.0.0.1", "", time.Second)

	_, err := client.Complete(context.Background(), port.ChatCompletionRequest{})

	require.ErrorContains(t, err, "chat_model")
}
