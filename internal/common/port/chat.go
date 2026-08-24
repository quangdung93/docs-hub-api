package port

import "context"

// ChatCompletionRequest là prompt đã được usecase giới hạn vào evidence nội bộ.
type ChatCompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
}

// ChatCompletionResult là nội dung cuối cùng, không chứa chain-of-thought.
type ChatCompletionResult struct {
	Content string
	Model   string
}

// ChatClient là capability sinh câu trả lời có căn cứ qua model cấu hình.
type ChatClient interface {
	Complete(ctx context.Context, input ChatCompletionRequest) (ChatCompletionResult, error)
}
