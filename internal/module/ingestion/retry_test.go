package ingestion

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/infrastructure/ai/ragflow"
)

// Sự cố có thật đã dẫn tới bản sửa này: worker poll trạng thái RAGFlow bị
// timeout một nhịp rồi đánh tài liệu 'failed' VĨNH VIỄN, trong khi RAGFlow bên
// kia đã parse xong. Đo lại thì độ trễ chỉ ~1s trên ngưỡng 30s.
func TestRetryable_LoiTamThoiThiThuLai(t *testing.T) {
	cases := map[string]error{
		"ctx hết hạn":           context.DeadlineExceeded,
		"ctx bị huỷ vì SIGTERM": context.Canceled,
		"lỗi bọc nhiều lớp":     fmt.Errorf("đọc trạng thái: %w", fmt.Errorf("gọi HTTP: %w", context.DeadlineExceeded)),
		"lỗi mạng":              &net.OpError{Op: "dial", Err: errors.New("connection refused")},
		"lỗi lạ chưa phân loại": errors.New("chuyện gì đó chưa từng gặp"),
		"RAGFlow 500":           &ragflow.APIError{HTTPStatus: 500, Retryable: true},
		"RAGFlow 429 quá tải":   &ragflow.APIError{HTTPStatus: 429, Retryable: true},
	}
	for ten, err := range cases {
		t.Run(ten, func(t *testing.T) {
			require.True(t, retryable(err))
		})
	}
}

func TestRetryable_LoiVinhVienThiDungHan(t *testing.T) {
	cases := map[string]error{
		// Đúng 4 file DOCX đã hỏng thật: .rels tham chiếu ảnh mà word/media/ rỗng.
		"DOCX hỏng": permanent(errors.New(
			`RAGFlow parse thất bại: There is no item named 'word/media/image22.png' in the archive`)),
		"mâu thuẫn dữ liệu": permanent(errors.New("project đã map tới RAGFlow dataset khác")),
		"RAGFlow 400":       &ragflow.APIError{HTTPStatus: 400, Retryable: false},
	}
	for ten, err := range cases {
		t.Run(ten, func(t *testing.T) {
			require.False(t, retryable(err))
		})
	}
}

// Lỗi vĩnh viễn phải giữ được dấu vết kể cả khi bị bọc thêm nhiều lớp fmt.Errorf
// trên đường trả về ProcessNext — nếu mất thì DOCX hỏng sẽ bị thử lại vô ích.
func TestRetryable_DauVinhVienSongQuaNhieuLopBoc(t *testing.T) {
	goc := permanent(errors.New("RAGFlow parse thất bại: file hỏng"))
	boc := fmt.Errorf("xử lý revision: %w", fmt.Errorf("chờ parse: %w", goc))
	require.False(t, retryable(boc))
}

func TestPermanent_GiuNguyenThongDiepGoc(t *testing.T) {
	goc := errors.New("RAGFlow parse thất bại: thiếu word/media/image1.png")
	err := permanent(goc)
	require.Equal(t, goc.Error(), err.Error(), "thông điệp ghi vào DB không được nhiễu chữ thừa")
	require.ErrorIs(t, err, goc, "phải unwrap được về lỗi gốc")
}

func TestPermanent_NilThiVanNil(t *testing.T) {
	require.NoError(t, permanent(nil))
	require.False(t, retryable(nil))
}

// Bảng quyết định của fail(): còn lượt + lỗi tạm thời thì xếp lại hàng đợi,
// ngược lại đánh hỏng. claim đã tăng attempt trước khi trả về nên Attempt là số
// lượt ĐÃ dùng — attempt=3/max=3 nghĩa là hết lượt.
func TestQuyetDinhThuLai(t *testing.T) {
	tamThoi := context.DeadlineExceeded
	vinhVien := permanent(errors.New("file hỏng"))
	cases := []struct {
		ten            string
		err            error
		attempt, toiDa int
		muonThuLai     bool
	}{
		{"lỗi tạm thời, lượt đầu", tamThoi, 1, 3, true},
		{"lỗi tạm thời, lượt cuối còn dư", tamThoi, 2, 3, true},
		{"lỗi tạm thời nhưng HẾT lượt", tamThoi, 3, 3, false},
		{"lỗi tạm thời, vượt quá lượt", tamThoi, 4, 3, false},
		{"lỗi vĩnh viễn ngay lượt đầu", vinhVien, 1, 3, false},
	}
	for _, c := range cases {
		t.Run(c.ten, func(t *testing.T) {
			require.Equal(t, c.muonThuLai, retryable(c.err) && c.attempt < c.toiDa)
		})
	}
}
