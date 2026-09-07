package usecase

import (
	"encoding/json"
	"fmt"
	"strings"
)

// uatPromptTemplate yêu cầu RAGFlow trả JSON thuần (không markdown) liệt kê
// test case UAT tổng hợp từ User Story/Acceptance Criteria trong tài liệu dự
// án — domain hiện tại không có entity User Story/AC riêng nên nội dung được
// LLM tổng hợp trực tiếp từ tài liệu đã ingest, không phải đọc từ DB.
const uatPromptTemplate = `Bạn là trợ lý phân tích tài liệu dự án phần mềm "%s".
Nhiệm vụ: đọc các tài liệu yêu cầu (User Story/Acceptance Criteria) đã nạp trong dự án
và liệt kê từng test case nghiệm thu (UAT) tương ứng.

CHỈ trả lời bằng JSON hợp lệ đúng schema sau, không thêm giải thích, không thêm
markdown code fence:
{"items":[{"title":"tên user story/chức năng",
"steps":"các bước thực hiện test cụ thể","expected":"kết quả mong đợi cụ thể",
"source":"tên tài liệu nguồn"}]}

Yêu cầu:
- Tối đa %d dòng, ưu tiên các chức năng quan trọng nhất nếu tài liệu có nhiều hơn.
- Nếu không tìm thấy User Story/AC nào, trả về {"items":[]}.
- steps và expected phải cụ thể, dựa trên nội dung THẬT của tài liệu, không bịa đặt.`

func uatPrompt(projectName string, maxItems int) string {
	return fmt.Sprintf(uatPromptTemplate, projectName, maxItems)
}

type uatRAGItem struct {
	Title    string `json:"title"`
	Steps    string `json:"steps"`
	Expected string `json:"expected"`
	Source   string `json:"source"`
}

type uatRAGResponse struct {
	Items []uatRAGItem `json:"items"`
}

// parseUATItems giải mã JSON RAGFlow trả về. LLM đôi khi vẫn bọc markdown code
// fence dù prompt đã yêu cầu không làm vậy — chủ động gỡ bỏ trước khi parse.
// Hạn chế đã biết: không validate được source khớp đúng tài liệu thật (khác
// mapRAGCitations của module chat) vì đây là 1 câu trả lời JSON gộp, không phải
// answer theo từng chunk có thể đối chiếu ngược.
func parseUATItems(content string) ([]uatRAGItem, error) {
	trimmed := stripCodeFence(content)
	var response uatRAGResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return nil, fmt.Errorf("decode JSON UAT items: %w", err)
	}
	return response.Items, nil
}

func stripCodeFence(content string) string {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}
