package usecase

import (
	"encoding/json"
	"fmt"
)

// Slot của từng milestone trong template Project Plan (sheet "Plan") — 5
// milestone cố định với số dòng task khác nhau (dòng 9/17/21/25/29 là header
// milestone có công thức MIN/MAX/SUM, các dòng còn lại là task). Số lượng
// milestone/task tối đa BỊ KHÓA bởi cấu trúc file mẫu, không thể mở rộng mà
// không phá công thức.
var planningMilestoneSlots = []struct { //nolint:gochecknoglobals // bảng tra cứu bất biến, khớp cấu trúc template
	headerRow    int
	firstTaskRow int
	maxTasks     int
}{
	{headerRow: 9, firstTaskRow: 10, maxTasks: 7},
	{headerRow: 17, firstTaskRow: 18, maxTasks: 3},
	{headerRow: 21, firstTaskRow: 22, maxTasks: 3},
	{headerRow: 25, firstTaskRow: 26, maxTasks: 3},
	{headerRow: 29, firstTaskRow: 30, maxTasks: 3},
}

const maxPlanningMilestones = 5

const planningPromptTemplate = `Bạn là trợ lý lập kế hoạch dự án phần mềm "%s".
Nhiệm vụ: đọc tài liệu yêu cầu/kế hoạch đã nạp trong dự án và tổng hợp thành kế
hoạch triển khai theo từng milestone/giai đoạn.

CHỈ trả lời bằng JSON hợp lệ đúng schema sau, không thêm giải thích, không thêm
markdown code fence:
{"milestones":[{"name":"tên milestone/giai đoạn",
"tasks":[{"name":"tên task","description":"mô tả công việc cụ thể"}]}]}

Yêu cầu:
- Tối đa 5 milestone, mỗi milestone tối đa 7 task.
- Nếu không tìm thấy thông tin kế hoạch nào, trả về {"milestones":[]}.
- name và description phải cụ thể, dựa trên nội dung THẬT của tài liệu, không bịa đặt.`

func planningPrompt(projectName string) string {
	return fmt.Sprintf(planningPromptTemplate, projectName)
}

type planningRAGTask struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type planningRAGMilestone struct {
	Name  string            `json:"name"`
	Tasks []planningRAGTask `json:"tasks"`
}

type planningRAGResponse struct {
	Milestones []planningRAGMilestone `json:"milestones"`
}

// parsePlanningMilestones giải mã JSON RAGFlow trả về, gỡ code fence lẫn chú
// thích trích dẫn [ID:n] trước khi parse (xem sanitizeRAGContent).
func parsePlanningMilestones(content string) ([]planningRAGMilestone, error) {
	trimmed := sanitizeRAGContent(content)
	var response planningRAGResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return nil, fmt.Errorf("decode JSON planning milestones: %w", err)
	}
	return response.Milestones, nil
}
