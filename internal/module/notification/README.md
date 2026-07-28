# Module `notification` (scaffold)

Chưa hiện thực. Khác với các module CRUD, đây chủ yếu là **consumer** message queue.

## Trách nhiệm
Lắng nghe sự kiện (ví dụ `user.created` do module user phát) và gửi email/thông báo.

## Điểm khác biệt kiến trúc
- Không (chỉ) là handler HTTP. Cần một consumer nền chạy cùng vòng đời app.
- Gợi ý: thêm `cmd/worker/` riêng, hoặc chạy consumer trong `bootstrap` như một background goroutine có graceful shutdown.

## Mã lỗi (templates/04)
- Nghiệp vụ: `NOTIFY_FAILED`.

## Port cần
- Consumer RabbitMQ (mở rộng `internal/infrastructure/mq/rabbitmq` với phần consume + auto-reconnect).
- Provider gửi email (thêm port `Mailer`).

## Lưu ý outbox
Hiện `user.created` được publish trong cùng transaction tạo user (at-most-once). Để đảm bảo không mất sự kiện, cân nhắc Outbox pattern (xem ADR-0005).
