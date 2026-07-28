// Package logger cung cấp logger có cấu trúc dựa trên Zap.
//
// Gói này KHÔNG phụ thuộc internal/* để có thể tái sử dụng ngoài repo.
// Việc gắn request_id/trace_id vào log do middleware đảm nhiệm (tạo child
// logger rồi WithContext), giữ cho logger không biết gì về HTTP.
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Options là tham số khởi tạo logger, do caller ánh xạ từ config sang.
type Options struct {
	Level    string // debug | info | warn | error
	Encoding string // json | console
	AppName  string
	Env      string
}

// New tạo *zap.Logger theo Options. Trả lỗi nếu level/encoding không hợp lệ.
func New(opts Options) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(opts.Level)
	if err != nil {
		return nil, fmt.Errorf("log level không hợp lệ %q: %w", opts.Level, err)
	}

	encCfg := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	switch opts.Encoding {
	case "console":
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	case "json", "":
		encoder = zapcore.NewJSONEncoder(encCfg)
	default:
		return nil, fmt.Errorf("log encoding không hợp lệ %q", opts.Encoding)
	}

	core := zapcore.NewCore(encoder, zapcore.Lock(zapcore.AddSync(stdout())), level)

	base := zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.Fields(
			zap.String("service", opts.AppName),
			zap.String("env", opts.Env),
		),
	)
	return base, nil
}
