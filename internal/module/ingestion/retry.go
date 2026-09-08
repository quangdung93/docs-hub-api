package ingestion

import (
	"errors"
	"time"
)

// dbWriteTimeout là hạn cho các lệnh ghi trạng thái job sau khi công việc đã
// hỏng. Cố tình ngắn: lúc worker đang tắt thì chỉ cần kịp ghi xong rồi thoát.
const dbWriteTimeout = 5 * time.Second

// retryFlag là hợp đồng tối thiểu để một lỗi tự khai báo có đáng thử lại không.
// Nhờ nó mà package này KHÔNG phải import tầng hạ tầng (ragflow) chỉ để nhận
// diện kiểu lỗi — *ragflow.APIError đã có sẵn phương thức này.
type retryFlag interface{ IsRetryable() bool }

// permanentError bọc một lỗi để đánh dấu "đừng thử lại nữa". Giữ nguyên chuỗi
// Error() của lỗi gốc để thông điệp ghi vào DB không bị nhiễu thêm chữ thừa.
type permanentError struct{ err error }

func (e permanentError) Error() string     { return e.err.Error() }
func (e permanentError) Unwrap() error     { return e.err }
func (e permanentError) IsRetryable() bool { return false }

// permanent đánh dấu lỗi là vĩnh viễn: thử lại chỉ tốn công. Dùng cho những
// trường hợp mà lần chạy sau chắc chắn cũng hỏng y hệt — ví dụ file DOCX tham
// chiếu tới ảnh không tồn tại trong gói zip.
func permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

// explicitlyPermanent chỉ nhận lỗi do CHÍNH code này gắn dấu bằng permanent().
//
// Khác retryable() ở chỗ nó KHÔNG hỏi cờ IsRetryable của lỗi bên ngoài, nên
// *ragflow.APIError dù mang cờ false vẫn không tính. Dùng cho nhánh cleanup:
// hỏng ở đó thì event nằm lại 'failed' vĩnh viễn, mà KHÔNG có API nào đưa nó về
// 'pending' — phải vào tận DB production chạy tay câu UPDATE. Nhánh ingest thì
// khác, người dùng còn bấm được POST retry.
//
// Vì cái giá lệch nhau như vậy nên chỗ này thà thử lại thừa 15 lượt còn hơn
// đánh hỏng nhầm. Đáng lưu ý: RAGFlow hay báo lỗi bằng HTTP 200 kèm code khác 0,
// mà decodeHTTPResponse gán cờ theo status, nên retryableStatus(200)=false —
// tức cả lớp lỗi ứng dụng của RAGFlow đều đang mang cờ "vĩnh viễn" dù nhiều cái
// chỉ là nhất thời.
func explicitlyPermanent(err error) bool {
	var marked permanentError
	return errors.As(err, &marked)
}

// retryable trả lời: gặp lỗi này thì nên xếp job lại hàng đợi hay đánh hỏng hẳn?
//
// Chỉ lỗi TỰ KHAI BÁO mình là vĩnh viễn mới bị đánh hỏng; còn lại mặc định thử
// lại. Chọn vậy vì cái giá của hai hướng sai lệch nhau rất xa: thử lại nhầm một
// lỗi vĩnh viễn thì tốn thêm vài lượt rồi cũng dừng (max_attempts chặn), còn
// đánh hỏng nhầm một lỗi tạm thời thì tài liệu chết vĩnh viễn dù RAGFlow bên
// kia đã xử lý xong — đúng sự cố đã xảy ra thật.
//
// Nhờ mặc định này mà timeout mạng, ctx bị huỷ do SIGTERM lúc deploy, hay lỗi
// 5xx nhất thời đều tự động được thử lại mà không phải liệt kê từng loại.
func retryable(err error) bool {
	if err == nil {
		return false
	}
	// *ragflow.APIError bật cờ cho 408/429/5xx và tắt cho 4xx còn lại;
	// permanentError luôn trả false.
	var flag retryFlag
	if errors.As(err, &flag) {
		return flag.IsRetryable()
	}
	return true
}
