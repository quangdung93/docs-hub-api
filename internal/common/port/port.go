// Package port khai báo các interface hạ tầng dùng chung giữa nhiều module.
//
// Đây là ranh giới của Clean Architecture: tầng usecase chỉ phụ thuộc các
// interface ở đây, còn implementation cụ thể (GORM, Redis, RabbitMQ, MinIO)
// nằm ở internal/infrastructure. Package này KHÔNG import bất kỳ thư viện hạ
// tầng nào — chỉ stdlib.
package port

import (
	"context"
	"time"
)

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

// ObjectStore là interface lưu trữ file (implement bằng MinIO/S3).
type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte, contentType string) (StoredObject, error)
	Get(ctx context.Context, key string) ([]byte, error)
	// PresignedGetURL tạo URL tải file XUỐNG có thời hạn.
	PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	// PresignedPutURL tạo URL tải file LÊN có thời hạn — dùng cho luồng client
	// (FE) tự upload thẳng lên storage, không đẩy file qua backend.
	PresignedPutURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Stat kiểm tra object đã thực sự tồn tại chưa (dùng để xác nhận client đã
	// upload xong qua presigned URL). Không tồn tại -> ErrObjectNotFound.
	Stat(ctx context.Context, key string) (StoredObject, error)
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
