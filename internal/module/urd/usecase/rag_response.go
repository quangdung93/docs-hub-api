package usecase

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
)

// noDataSnippetLimit giới hạn phần nội dung đính kèm vào log — xem giải thích
// đầy đủ ở report/usecase/rag_response.go, copy-adapt cho module urd.
const noDataSnippetLimit = 200

// citationMarkerPattern/sanitizeRAGContent/noDataAnswer/noDataError: copy-adapt
// từ report/usecase/{uat_prompt.go,rag_response.go} — cùng lý do (RAGFlow có
// thể trả văn xuôi "không tìm thấy dữ liệu" hoặc bọc markdown code fence dù
// prompt đã yêu cầu không làm vậy). Không tái dùng thẳng module report vì đó
// là chi tiết triển khai nội bộ (hàm không export), không phải hợp đồng dùng
// chung giữa các module.
var citationMarkerPattern = regexp.MustCompile(`(?i)\[\s*id\s*:\s*[\d\s,]+\]`)

func sanitizeRAGContent(content string) string {
	trimmed := citationMarkerPattern.ReplaceAllString(content, "")
	trimmed = strings.TrimSpace(trimmed)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}

func noDataAnswer(content string) bool {
	return !strings.HasPrefix(sanitizeRAGContent(content), "{")
}

func noDataError(content string) error {
	snippet := []rune(sanitizeRAGContent(content))
	if len(snippet) > noDataSnippetLimit {
		snippet = snippet[:noDataSnippetLimit]
	}
	return apperr.BadRequest("Tài liệu URD chưa đủ dữ liệu để phân tích edge case").
		WithCause(fmt.Errorf("RAGFlow trả văn xuôi thay vì JSON: %q", string(snippet)))
}
