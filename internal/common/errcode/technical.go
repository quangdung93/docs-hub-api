package errcode

import "net/http"

// Mã lỗi KỸ THUẬT — trả về HTTP 4xx/5xx (do middleware xử lý).
// Danh sách theo bảng templates/04.
const (
	Success         = "GEN_200"          // Thao tác thành công
	Req400          = "REQ_400"          // Request sai định dạng, thiếu field
	Auth401         = "AUTH_401"         // Chưa xác thực / thiếu token
	Auth403         = "AUTH_403"         // Không có quyền truy cập
	NotFound        = "NOT_FOUND"        // Resource không tồn tại (generic)
	UserNotFound    = "USR_404"          // Không tìm thấy người dùng
	ReqTimeout      = "REQ_TIMEOUT"      // Yêu cầu xử lý quá lâu (408)
	UpgradeRequired = "UPGRADE_REQUIRED" // Cần nâng cấp phiên bản client (426)
	Rate429         = "RATE_429"         // Gửi quá nhiều request (429)
	Sys500          = "SYS_500"          // Lỗi hệ thống (null/crash bất thường)
	DB500           = "DB_500"           // Lỗi kết nối hoặc xử lý database
	DB503           = "DB_503"           // Database quá tải / không sẵn sàng
	MQ502           = "MQ_502"           // Lỗi message queue
	Ext504          = "EXT_504"          // Timeout khi gọi hệ thống bên ngoài
)

// technicalStatus ánh xạ mã kỹ thuật -> HTTP status.
// Không export map trực tiếp (tránh bị sửa ngoài ý muốn); dùng HTTPStatusOf.
var technicalStatus = map[string]int{ //nolint:gochecknoglobals // bảng tra cứu bất biến
	Success:         http.StatusOK,
	Req400:          http.StatusBadRequest,
	Auth401:         http.StatusUnauthorized,
	Auth403:         http.StatusForbidden,
	NotFound:        http.StatusNotFound,
	UserNotFound:    http.StatusNotFound,
	ReqTimeout:      http.StatusRequestTimeout,
	UpgradeRequired: http.StatusUpgradeRequired,
	Rate429:         http.StatusTooManyRequests,
	Sys500:          http.StatusInternalServerError,
	DB500:           http.StatusInternalServerError,
	DB503:           http.StatusServiceUnavailable,
	MQ502:           http.StatusBadGateway,
	Ext504:          http.StatusGatewayTimeout,
}

// HTTPStatusOf trả về HTTP status của mã kỹ thuật. Mã lạ -> 500 (an toàn).
func HTTPStatusOf(code string) int {
	if s, ok := technicalStatus[code]; ok {
		return s
	}
	return http.StatusInternalServerError
}
