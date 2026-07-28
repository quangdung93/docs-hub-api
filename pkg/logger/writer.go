package logger

import (
	"io"
	"os"
)

// stdout tách ra hàm riêng để test có thể thay thế nếu cần
// (đồng thời tránh biến global khiến gochecknoglobals báo lỗi).
func stdout() io.Writer {
	return os.Stdout
}
