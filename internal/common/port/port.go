// Package port khai báo các interface hạ tầng dùng chung giữa nhiều module.
//
// Đây là ranh giới của Clean Architecture: tầng usecase chỉ phụ thuộc các
// interface ở đây, còn implementation cụ thể (GORM, Redis, RabbitMQ, MinIO)
// nằm ở internal/infrastructure. Package này KHÔNG import bất kỳ thư viện hạ
// tầng nào — chỉ stdlib.
package port

import (
	"context"
	"errors"
	"io"
	"time"
)

// AuditEvent là bản ghi hành động bảo mật/nghiệp vụ dùng chung giữa delivery adapter.
type AuditEvent struct {
	ActorUserID string
	ProjectID   string
	Action      string
	EntityType  string
	EntityID    string
	RequestID   string
	Metadata    map[string]any
}

// Auditor ghi audit bền vững; caller không phụ thuộc database cụ thể.
type Auditor interface {
	Record(ctx context.Context, event AuditEvent) error
}

// TxManager trừu tượng hóa transaction. Usecase gọi Do và chạy toàn bộ thao tác
// trong callback; implementation nhét handle transaction vào context để các
// repository tự động tham gia (usecase không bao giờ thấy *gorm.DB).
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// Cache là interface cache key-value (implement bằng Redis).
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	// Incr tăng bộ đếm và trả về giá trị mới; dùng cho rate limit.
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// ErrCacheMiss được Cache.Get trả về khi key không tồn tại.
// (định nghĩa ở đây để usecase phân biệt "không có" với "lỗi hạ tầng").
var ErrCacheMiss = cacheMissError{}

type cacheMissError struct{}

func (cacheMissError) Error() string { return "cache: key không tồn tại" }

// Event là thông điệp publish lên message queue.
type Event struct {
	// RoutingKey ví dụ "user.created".
	RoutingKey string
	// Body là payload đã serialize (thường JSON).
	Body []byte
	// Headers tùy chọn (ví dụ trace context).
	Headers map[string]string
}

// Publisher là interface phát sự kiện (implement bằng RabbitMQ).
type Publisher interface {
	Publish(ctx context.Context, evt Event) error
}

// StoredObject mô tả object khi upload.
type StoredObject struct {
	Key         string
	Size        int64
	ContentType string
}

// ErrPresignUnsupported báo storage backend không hỗ trợ presigned URL.
// Filesystem local dùng upload/download qua API đã xác thực thay vì URL S3.
var ErrPresignUnsupported = errors.New("object store không hỗ trợ presigned URL")

// ObjectStore là interface lưu trữ file (implement bằng filesystem hoặc MinIO/S3).
type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte, contentType string) (StoredObject, error)
	// PutReader tải dữ liệu theo luồng để không giữ toàn bộ file lớn trong RAM.
	PutReader(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (StoredObject, error)
	Get(ctx context.Context, key string) ([]byte, error)
	// GetReader đọc object theo luồng để kiểm tra hash hoặc phục vụ parser.
	GetReader(ctx context.Context, key string) (io.ReadCloser, error)
	// Stat trả metadata hiện tại để xác minh upload trực tiếp trước khi tạo revision.
	Stat(ctx context.Context, key string) (StoredObject, error)
	// PresignedPutURL tạo URL upload trực tiếp có thời hạn.
	PresignedPutURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	// PresignedGetURL tạo URL tải file có thời hạn.
	PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
}

// ErrObjectNotFound được ObjectStore.Stat trả về khi object chưa tồn tại
// (ví dụ: client xin presigned URL nhưng chưa/không upload xong).
var ErrObjectNotFound = objectNotFoundError{}

type objectNotFoundError struct{}

func (objectNotFoundError) Error() string { return "object store: object không tồn tại" }

// Clock trừu tượng hóa thời gian để usecase test được (không gọi time.Now trực tiếp).
type Clock interface {
	Now() time.Time
}

// HealthChecker là interface kiểm tra sức khỏe một dependency (dùng cho /readyz).
type HealthChecker interface {
	// Name trả về tên dependency (ví dụ "postgres").
	Name() string
	// Check trả về nil nếu healthy, error nếu không.
	Check(ctx context.Context) error
}
