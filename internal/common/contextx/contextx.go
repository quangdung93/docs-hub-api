// Package contextx quản lý các giá trị xuyên suốt request trong context:
// request_id, trace_id và actor (thông tin người dùng đã xác thực).
//
// Dùng key kiểu riêng để tránh va chạm, và chỉ expose qua hàm getter/setter.
package contextx

import "context"

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyTraceID
	keyActor
)

// Actor là thông tin chủ thể thực hiện request (rút gọn từ JWT claims).
type Actor struct {
	UserID string
	Email  string
	Roles  []string
}

// HasRole kiểm tra actor có vai trò role không.
func (a Actor) HasRole(role string) bool {
	for _, r := range a.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// WithRequestID gắn request_id vào context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestID lấy request_id (rỗng nếu chưa có).
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(keyRequestID).(string); ok {
		return v
	}
	return ""
}

// WithTraceID gắn trace_id vào context (thường lấy từ span OpenTelemetry).
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyTraceID, id)
}

// TraceID lấy trace_id (rỗng nếu chưa có).
func TraceID(ctx context.Context) string {
	if v, ok := ctx.Value(keyTraceID).(string); ok {
		return v
	}
	return ""
}

// WithActor gắn actor đã xác thực vào context.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, keyActor, a)
}

// ActorFrom lấy actor từ context. ok=false nếu request chưa xác thực.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(keyActor).(Actor)
	return a, ok
}
