package logger

import (
	"context"

	"go.uber.org/zap"
)

// ctxKey là kiểu riêng để tránh va chạm key trong context.
type ctxKey struct{}

// WithContext gắn logger vào context. Middleware dùng để truyền child logger
// (đã kèm request_id, trace_id) xuống các tầng dưới.
func WithContext(ctx context.Context, l *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext lấy logger từ context. Nếu không có, trả về No-op logger để
// caller không bao giờ phải kiểm tra nil.
func FromContext(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok && l != nil {
		return l
	}
	return zap.NewNop()
}
