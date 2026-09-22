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
{"cases":[{"description":"mô tả cụ thể 1 edge case chưa được đề cập","target_heading":"tiêu đề mục nên bổ sung nội dung xử lý case này"}]}

Yêu cầu:
- Trả lời bằng tiếng Việt.
- Tối đa %d edge case, ưu tiên các trường hợp quan trọng/rủi ro cao nhất.
- Mỗi description phải cụ thể, bám sát nội dung THẬT của tài liệu, không bịa đặt.
- target_heading phải COPY NGUYÊN VĂN một dòng tiêu đề ĐANG CÓ trong tài liệu
  trên (các dòng bắt đầu bằng dấu "#"), bỏ phần dấu "#" và khoảng trắng đầu
  dòng. Chọn đúng mục mà nội dung xử lý case này nên được bổ sung vào — ví dụ
  mục tiêu chí chấp nhận (AC) hoặc quy tắc nghiệp vụ (BR) của chức năng liên
  quan.
- TUYỆT ĐỐI KHÔNG tự nghĩ ra tiêu đề mới, không sửa chữ trong tiêu đề. Nếu
  không xác định chắc chắn case thuộc mục nào, để target_heading là chuỗi rỗng "".
- Nếu tài liệu đã đủ chi tiết, không còn edge case đáng chú ý nào, trả về {"cases":[]}.`

func urdPrompt(documentTitle, canonicalText string) string {
	if len([]rune(canonicalText)) > maxCanonicalTextChars {
		canonicalText = string([]rune(canonicalText)[:maxCanonicalTextChars])
	}
	return fmt.Sprintf(urdPromptTemplate, documentTitle, canonicalText, maxEdgeCases)
}

type urdRAGCase struct {
	Description   string `json:"description"`
	TargetHeading string `json:"target_heading"`
}

type urdRAGResponse struct {
	Cases []urdRAGCase `json:"cases"`
}

// parsedCase là 1 edge case AI liệt kê: mô tả + tiêu đề mục AI cho rằng nội
// dung xử lý nên được bổ sung vào.
//
// targetHeading chỉ là ĐỀ XUẤT: docxmerge còn phải đối chiếu nó với tiêu đề
// thật trong word/document.xml mới chèn thẳng vào mục đó; không khớp thì lùi
// về phụ lục cuối tài liệu (xem docxmerge/placement.go).
type parsedCase struct {
	description   string
	targetHeading string
}

// parseEdgeCases giải mã JSON RAGFlow trả về thành danh sách edge case.
func parseEdgeCases(content string) ([]parsedCase, error) {
	trimmed := sanitizeRAGContent(content)
	var response urdRAGResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return nil, fmt.Errorf("decode JSON edge case: %w", err)
	}
	out := make([]parsedCase, 0, len(response.Cases))
	for _, c := range response.Cases {
		if c.Description != "" {
			out = append(out, parsedCase{description: c.Description, targetHeading: c.TargetHeading})
		}
	}
	if len(out) > maxEdgeCases {
		out = out[:maxEdgeCases]
	}
	return out, nil
}
