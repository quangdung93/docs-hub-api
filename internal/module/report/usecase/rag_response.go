package usecase

import (
	"fmt"
	"strings"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
)

// noDataSnippetLimit giới hạn phần nội dung đính kèm vào log — đủ để nhận ra câu
// trả lời, không đủ để làm ngập log khi RAGFlow trả về cả một đoạn dài.
const noDataSnippetLimit = 200

// noDataAnswer nhận biết RAGFlow trả về văn xuôi thay vì JSON.
//
// Cả ba prompt đều yêu cầu LLM chỉ trả JSON, nên nội dung không mở đầu bằng "{"
// nghĩa là câu trả lời không dùng được. Hai nguồn đã biết, và cả hai đều đến từ
// cấu hình MẶC ĐỊNH của chat assistant chứ không phải sự cố hạ tầng:
//
//   - prompt_config.system mặc định buộc câu trả lời phải chứa
//     "The answer you are looking for is not found in the dataset!" khi không
//     truy hồi được đoạn tài liệu nào liên quan;
//   - prompt_config.empty_response mặc định là
//     "Sorry! No relevant content was found in the knowledge base!".
//
// Trước đây cả hai rơi vào nhánh JSON hỏng và bị báo thành EXT_504 — người dùng
// đọc thành "hệ thống ngoài đang chết", trong khi thực tế chỉ là tài liệu dự án
// chưa đủ dữ liệu.
//
// Chỉ chặn trường hợp văn xuôi. Nội dung mở đầu bằng "{" mà vẫn parse hỏng thì
// đúng là JSON méo — giữ nguyên nhánh lỗi kỹ thuật để còn điều tra.
func noDataAnswer(content string) bool {
	return !strings.HasPrefix(sanitizeRAGContent(content), "{")
}

// noDataError dựng lỗi cho trường hợp trên. Thông điệp gửi người dùng nói đúng
// điều app biết chắc — không sinh được báo cáo từ tài liệu hiện có — chứ không
// đoán là "thiếu User Story" hay "RAGFlow chết", vì cả hai đều có thể sai.
// Câu trả lời thật đi kèm vào cause để còn lần lại trong log.
func noDataError(content string) error {
	snippet := []rune(sanitizeRAGContent(content))
	if len(snippet) > noDataSnippetLimit {
		return apperr.BadRequest("Tài liệu dự án chưa đủ dữ liệu để sinh báo cáo").
			WithCause(fmt.Errorf("RAGFlow trả văn xuôi thay vì JSON: %q…",
				string(snippet[:noDataSnippetLimit])))
	}
	return apperr.BadRequest("Tài liệu dự án chưa đủ dữ liệu để sinh báo cáo").
		WithCause(fmt.Errorf("RAGFlow trả văn xuôi thay vì JSON: %q", string(snippet)))
}
