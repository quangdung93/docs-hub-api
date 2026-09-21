package usecase

import "strings"

// noActionResolutions liệt kê các câu trả lời "không có nội dung nghiệp vụ
// cần bổ sung vào URD" thường gặp (vd trả lời cho câu hỏi edge case kiểu
// "hệ thống có cơ chế X hay không?" bằng "Không", hoặc "chưa có tính năng
// này"). Dùng để TỰ ĐỘNG loại case khỏi phụ lục URD mới khi client KHÔNG gửi
// include_in_document tường minh — cho phép tính năng hoạt động được mà
// không cần FE đổi gì.
//
// CỐ Ý so khớp CHÍNH XÁC theo danh sách cố định (không suy luận ngữ nghĩa
// bằng AI) để hành vi loại trừ luôn xác định, dò lại được, và review được
// trong code thay vì phụ thuộc 1 lời gọi RAGFlow không tất định.
//
// Rủi ro đã biết: 1 câu trả lời TRÙNG chữ với danh sách này nhưng thực chất
// là 1 quyết định nghiệp vụ thật (vd đáp "Không" cho câu "có tự động gia hạn
// hay không?" — "Không" ở đây LÀ nội dung cần đưa vào URD) vẫn bị loại nhầm.
// Khi FE thêm được lựa chọn tường minh cho người dùng, gửi
// include_in_document sẽ luôn được ưu tiên, bỏ qua suy đoán ở đây (xem
// applyResolutions).
var noActionResolutions = map[string]bool{ //nolint:gochecknoglobals // bảng tra cứu bất biến
	"không":                 true,
	"không có":              true,
	"không có gì":           true,
	"không áp dụng":         true,
	"không hỗ trợ":          true,
	"chưa hỗ trợ":           true,
	"chưa có":               true,
	"chưa có chức năng":     true,
	"chưa có chức năng này": true,
	"chưa có tính năng":     true,
	"chưa có tính năng này": true,
	"không cần thiết":       true,
	"không xử lý":           true,
	"chưa xử lý":            true,
	"không thực hiện":       true,
	"n/a":                   true,
	"na":                    true,
}

// isNoActionResolution báo hướng giải quyết có khớp 1 trong các câu trả lời
// "không có nội dung cần bổ sung" đã biết hay không — sau khi chuẩn hoá
// (thường/hoa, khoảng trắng đầu-cuối, dấu câu cuối câu).
func isNoActionResolution(resolution string) bool {
	normalized := strings.ToLower(strings.TrimSpace(resolution))
	normalized = strings.TrimRight(normalized, ".!?,;: ")
	return noActionResolutions[normalized]
}
