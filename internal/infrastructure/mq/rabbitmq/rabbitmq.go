// Package rabbitmq cung cấp connection RabbitMQ, publisher (implement
// port.Publisher) và health checker.
//
// Lưu ý phạm vi: đây là bản boilerplate tối giản nhưng thực dụng — publish có
// confirm, tự mở lại channel khi lỗi. Consumer nền và reconnect nâng cao để
// dành cho module notification (xem README).
package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

// Config là tham số kết nối.
type Config struct {
	URL      string
	Exchange string
}

// Connection bọc kết nối AMQP và exchange mặc định.
type Connection struct {
	conn     *amqp.Connection
	exchange string
}

// New mở kết nối và khai báo exchange (topic, durable).
func New(cfg Config) (*Connection, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("kết nối RabbitMQ thất bại: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mở channel khai báo exchange thất bại: %w", err)
	}
	defer func() { _ = ch.Close() }()

	if err := ch.ExchangeDeclare(cfg.Exchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("khai báo exchange %q thất bại: %w", cfg.Exchange, err)
	}

	return &Connection{conn: conn, exchange: cfg.Exchange}, nil
}

// Close đóng kết nối.
func (c *Connection) Close() error {
	if c.conn == nil || c.conn.IsClosed() {
		return nil
	}
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("đóng RabbitMQ: %w", err)
	}
	return nil
}

// Publisher implement port.Publisher với publisher-confirm.
type Publisher struct {
	conn *Connection
	mu   sync.Mutex // amqp.Channel không an toàn cho concurrent publish
}

// NewPublisher tạo publisher từ connection.
func NewPublisher(conn *Connection) *Publisher {
	return &Publisher{conn: conn}
}

// Publish phát event lên exchange với routing key. Lỗi -> caller bọc thành MQ_502.
func (p *Publisher) Publish(ctx context.Context, evt port.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch, err := p.conn.conn.Channel()
	if err != nil {
		return fmt.Errorf("mở channel publish thất bại: %w", err)
	}
	defer func() { _ = ch.Close() }()

	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("bật publisher confirm thất bại: %w", err)
	}

	headers := amqp.Table{}
	for k, v := range evt.Headers {
		headers[k] = v
	}

	conf, err := ch.PublishWithDeferredConfirmWithContext(ctx, p.conn.exchange, evt.RoutingKey,
		true,  // mandatory: không route được -> lỗi
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         evt.Body,
			Headers:      headers,
			DeliveryMode: amqp.Persistent,
		},
	)
	if err != nil {
		return fmt.Errorf("publish %q thất bại: %w", evt.RoutingKey, err)
	}
	if !conf.Wait() {
		return fmt.Errorf("broker không xác nhận (nack) event %q", evt.RoutingKey)
	}
	return nil
}
