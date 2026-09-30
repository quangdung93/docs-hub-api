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
	"không hỗ trợ":          true,
	"chưa hỗ trợ":           true,
	"chưa có":               true,
	"chưa có chức năng":     true,
	"chưa có chức năng này": true,
	"chưa có tính năng":     true,
	"chưa có tính năng này": true,
	"không cần thiết":       true,
	"không thực hiện":       true,
	"n/a":                   true,
	"na":                    true,
	"không cần":             true,
	"không ảnh hưởng":       true,
	"skip":                  true,
	"ignore":                true,
}

// noActionPrefixes là các cụm mang nghĩa "không bổ sung gì" — đứng một mình
// hoặc khi người dùng ghi thêm lý do phía sau, vd "Bỏ qua do lỗi gửi thông
// báo". Nhóm "không/chưa có hướng giải quyết" là case đã được xem xét nhưng
// không có nội dung nào để ghi nhận vào URD. CHỈ gồm cụm không
// thể mở đầu một quyết định nghiệp vụ; "không" bị loại có chủ đích vì "Không,
// hệ thống khoá tài khoản" là nội dung thật cần đưa vào URD.
var noActionPrefixes = []string{ //nolint:gochecknoglobals // bảng tra cứu bất biến
	"bỏ qua", "không áp dụng", "không xử lý", "chưa xử lý", "không cần xử lý",
	"ngoài phạm vi", "hành vi người dùng", "chấp nhận rủi ro", "out of scope",
	"không có hướng giải quyết", "chưa có hướng giải quyết", "không có hướng xử lý",
	"chưa có hướng xử lý", "không có giải pháp", "chưa có giải pháp",
}

// noActionReasonSeparators là cách người dùng nối lý do sau cụm "không xử lý".
var noActionReasonSeparators = []string{ //nolint:gochecknoglobals // bảng tra cứu bất biến
	" do ", " vì ", " bởi ", " (", ", ", " - ", " – ", ": ",
}

// isNoActionResolution báo hướng giải quyết có khớp 1 trong các câu trả lời
// "không có nội dung cần bổ sung" đã biết hay không — sau khi chuẩn hoá
// (thường/hoa, khoảng trắng thừa, dấu câu cuối câu) — hoặc là 1 cụm trong
// noActionPrefixes kèm lý do phía sau.
func isNoActionResolution(resolution string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(resolution), " "))
	normalized = strings.TrimRight(normalized, ".!?,;: ")
	if noActionResolutions[normalized] {
		return true
	}
	for _, prefix := range noActionPrefixes {
		rest, ok := strings.CutPrefix(normalized, prefix)
		if !ok {
			continue
		}
		if rest == "" {
			return true
		}
		for _, sep := range noActionReasonSeparators {
			if strings.HasPrefix(rest, sep) {
				return true
			}
		}
	}
	return false
}
