package usecase

import (
	"encoding/json"
	"fmt"
)

// maxTestcaseItems giới hạn số dòng report — sheet "Test Cases" của template
// hỗ trợ tới dòng 500 (bắt đầu dòng 7), nhưng giới hạn thấp hơn để prompt/
// response RAGFlow không quá dài và file vẫn gọn để review.
const maxTestcaseItems = 150

const testcasePromptTemplate = `Bạn là trợ lý kiểm thử phần mềm cho dự án "%s".
Nhiệm vụ: đọc tài liệu yêu cầu (User Story/Acceptance Criteria) đã nạp trong dự
án và sinh danh sách test case chi tiết tương ứng.

CHỈ trả lời bằng JSON hợp lệ đúng schema sau, không thêm giải thích, không thêm
markdown code fence:
{"items":[{"req_id":"mã yêu cầu liên quan (nếu có)",
"doc_source":"tên tài liệu nguồn","group":"Functional hoặc UI hoặc Non-functional",
"priority":"High hoặc Medium hoặc Low","title":"tên test case ngắn gọn",
"precondition":"điều kiện/dữ liệu tiền đề","steps":"các bước thực hiện cụ thể",
"expected":"kết quả mong đợi cụ thể"}]}

Yêu cầu:
- Tối đa %d dòng, ưu tiên các chức năng quan trọng nhất nếu tài liệu có nhiều hơn.
- Nếu không tìm thấy User Story/AC nào, trả về {"items":[]}.
- steps và expected phải cụ thể, dựa trên nội dung THẬT của tài liệu, không bịa đặt.`

func testcasePrompt(projectName string, maxItems int) string {
	return fmt.Sprintf(testcasePromptTemplate, projectName, maxItems)
}

type testcaseRAGItem struct {
	ReqID        string `json:"req_id"`
	DocSource    string `json:"doc_source"`
	Group        string `json:"group"`
	Priority     string `json:"priority"`
	Title        string `json:"title"`
	Precondition string `json:"precondition"`
	Steps        string `json:"steps"`
	Expected     string `json:"expected"`
}

type testcaseRAGResponse struct {
	Items []testcaseRAGItem `json:"items"`
}

// parseTestcaseItems giải mã JSON RAGFlow trả về, gỡ code fence lẫn chú thích
// trích dẫn [ID:n] trước khi parse (xem sanitizeRAGContent).
func parseTestcaseItems(content string) ([]testcaseRAGItem, error) {
	trimmed := sanitizeRAGContent(content)
	var response testcaseRAGResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return nil, fmt.Errorf("decode JSON testcase items: %w", err)
	}
	return response.Items, nil
}
