package logger

import (
	"io"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

// stdout tách ra hàm riêng để test có thể thay thế nếu cần
// (đồng thời tránh biến global khiến gochecknoglobals báo lỗi).
func stdout() io.Writer {
	return os.Stdout
}

// fileWriter tạo writer ghi log ra file, tự xoay vòng (rotate) theo dung
// lượng/tuổi file để tránh phình ổ đĩa khi chạy dài ngày.
func fileWriter(opts Options) io.Writer {
	return &lumberjack.Logger{
		Filename:   opts.FilePath,
		MaxSize:    opts.MaxSizeMB,
		MaxBackups: opts.MaxBackups,
		MaxAge:     opts.MaxAgeDays,
		Compress:   opts.Compress,
	}
}

// resolveWriter chọn đích ghi log theo Options.Output.
// "" hoặc "stdout" giữ hành vi mặc định trước đây (chỉ in ra console).
func resolveWriter(opts Options) io.Writer {
	switch opts.Output {
	case "file":
		return fileWriter(opts)
	case "both":
		return io.MultiWriter(stdout(), fileWriter(opts))
	default:
		return stdout()
	}
}
