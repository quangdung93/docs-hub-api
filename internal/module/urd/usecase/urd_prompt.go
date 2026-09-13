package usecase

import (
	"encoding/json"
	"fmt"
)

// maxEdgeCases giới hạn số case AI liệt kê — tránh 1 phiên bản URD mới bị
// phình quá lớn vì AI liệt kê tràn lan.
const maxEdgeCases = 30

// maxCanonicalTextChars giới hạn độ dài văn bản nhúng thẳng vào prompt (khác
// report: report dựa vào RAGFlow tự truy hồi theo câu hỏi, còn ở đây ta nhúng
// TOÀN VĂN tài liệu URD vì cần AI đọc hết để tìm chỗ còn thiếu edge case).
const maxCanonicalTextChars = 40000

// urdPromptTemplate yêu cầu RAGFlow đọc toàn văn tài liệu URD (nhúng thẳng
// trong prompt, không qua truy hồi RAG) và liệt kê edge case chưa được đề cập
// — URD v1.2 mục XI.
const urdPromptTemplate = `Bạn là trợ lý phân tích tài liệu URD (User Requirement Document) của dự án phần mềm "%s".

Dưới đây là toàn bộ nội dung tài liệu URD (đã trích xuất thành văn bản thuần):
---
%s
---

Nhiệm vụ: đọc kỹ tài liệu trên và liệt kê các "edge case" (trường hợp biên,
tình huống ngoại lệ) liên quan tới các yêu cầu/luồng nghiệp vụ đã mô tả nhưng
CHƯA được tài liệu đề cập cách xử lý.

CHỈ trả lời bằng JSON hợp lệ đúng schema sau, không thêm giải thích, không
thêm markdown code fence:
{"cases":[{"description":"mô tả cụ thể 1 edge case chưa được đề cập"}]}

Yêu cầu:
- Trả lời bằng tiếng Việt.
- Tối đa %d edge case, ưu tiên các trường hợp quan trọng/rủi ro cao nhất.
- Mỗi description phải cụ thể, bám sát nội dung THẬT của tài liệu, không bịa đặt.
- Nếu tài liệu đã đủ chi tiết, không còn edge case đáng chú ý nào, trả về {"cases":[]}.`

func urdPrompt(documentTitle, canonicalText string) string {
	if len([]rune(canonicalText)) > maxCanonicalTextChars {
		canonicalText = string([]rune(canonicalText)[:maxCanonicalTextChars])
	}
	return fmt.Sprintf(urdPromptTemplate, documentTitle, canonicalText, maxEdgeCases)
}

type urdRAGCase struct {
	Description string `json:"description"`
}

type urdRAGResponse struct {
	Cases []urdRAGCase `json:"cases"`
}

// parseEdgeCases giải mã JSON RAGFlow trả về thành danh sách mô tả edge case.
func parseEdgeCases(content string) ([]string, error) {
	trimmed := sanitizeRAGContent(content)
	var response urdRAGResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return nil, fmt.Errorf("decode JSON edge case: %w", err)
	}
	out := make([]string, 0, len(response.Cases))
	for _, c := range response.Cases {
		if c.Description != "" {
			out = append(out, c.Description)
		}
	}
	if len(out) > maxEdgeCases {
		out = out[:maxEdgeCases]
	}
	return out, nil
}
