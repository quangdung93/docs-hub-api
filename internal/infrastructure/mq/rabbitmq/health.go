package rabbitmq

import (
	"context"
	"errors"
)

// healthChecker kiểm tra kết nối RabbitMQ cho readiness.
type healthChecker struct {
	conn *Connection
}

// NewHealthChecker tạo checker sức khỏe RabbitMQ (implement port.HealthChecker).
func NewHealthChecker(conn *Connection) *healthChecker {
	return &healthChecker{conn: conn}
}

func (h *healthChecker) Name() string { return "rabbitmq" }

// Check khẳng định kết nối chưa đóng. amqp không có Ping, dùng IsClosed.
func (h *healthChecker) Check(_ context.Context) error {
	if h.conn == nil || h.conn.conn == nil || h.conn.conn.IsClosed() {
		return errors.New("kết nối RabbitMQ đã đóng")
	}
	return nil
}
