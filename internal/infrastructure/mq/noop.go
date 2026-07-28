// Package mq chứa tiện ích message queue dùng chung.
package mq

import (
	"context"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

// NoopPublisher là publisher không làm gì, dùng khi RabbitMQ bị tắt (enabled=false).
// Nhờ đó usecase vẫn chạy được mà không cần điều kiện hóa việc publish.
type NoopPublisher struct{}

// Publish bỏ qua event và trả nil.
func (NoopPublisher) Publish(_ context.Context, _ port.Event) error { return nil }
