// Package minio cung cấp client MinIO (S3-compatible) và implement
// port.ObjectStore + health checker.
//
// Phạm vi: client + health check + thao tác cơ bản (put/get/presign/delete).
// Đây là điểm nối (seam) cho module file trong tương lai (xem README).
package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

// Config là tham số kết nối MinIO.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	Region    string

	// PublicEndpoint/PublicUseSSL: host công khai dùng để KÝ presigned URL.
	// Xem config.MinIOConfig để biết vì sao không thể ký bằng host nội bộ.
	PublicEndpoint string
	PublicUseSSL   bool
}

// New tạo client và đảm bảo bucket tồn tại.
func New(ctx context.Context, cfg Config) (*minio.Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("khởi tạo client MinIO thất bại: %w", err)
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("kiểm tra bucket %q thất bại: %w", cfg.Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("tạo bucket %q thất bại: %w", cfg.Bucket, err)
		}
	}
	return client, nil
}

// NewPresign tạo client CHỈ dùng để ký presigned URL, trỏ vào PublicEndpoint.
//
// Hàm này không mở kết nối mạng nào — minio.New chỉ dựng struct — nên gọi được
// kể cả khi host công khai chưa có DNS. Trả về nil khi không cấu hình public
// endpoint (hoặc trùng endpoint nội bộ): khi đó Store ký bằng client chính.
func NewPresign(cfg Config) (*minio.Client, error) {
	if cfg.PublicEndpoint == "" || cfg.PublicEndpoint == cfg.Endpoint {
		return nil, nil
	}
	client, err := minio.New(cfg.PublicEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.PublicUseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("khởi tạo client ký presigned URL thất bại: %w", err)
	}
	return client, nil
}

// Store implement port.ObjectStore.
type Store struct {
	client *minio.Client
	// presign ký các URL mà client bên ngoài sẽ gọi. Thường trỏ host công khai;
	// bằng client khi không cấu hình PublicEndpoint.
	presign *minio.Client
	bucket  string
}

var _ port.ObjectStore = (*Store)(nil)

// NewStore bọc client thành port.ObjectStore. Presigned URL ký bằng chính
// client này — chỉ dùng được khi endpoint đã là host mà client ngoài gọi tới.
func NewStore(client *minio.Client, bucket string) *Store {
	return &Store{client: client, presign: client, bucket: bucket}
}

// NewStoreWithPresign tách đường ký presigned URL khỏi đường kết nối nội bộ:
// thao tác put/get/stat đi qua client, còn URL trả cho client ngoài được ký
// bằng presign. Truyền presign nil thì rơi về hành vi của NewStore.
func NewStoreWithPresign(client, presign *minio.Client, bucket string) *Store {
	if presign == nil {
		presign = client
	}
	return &Store{client: client, presign: presign, bucket: bucket}
}

// Put tải object lên bucket.
func (s *Store) Put(ctx context.Context, key string, data []byte, contentType string) (port.StoredObject, error) {
	return s.PutReader(ctx, key, bytes.NewReader(data), int64(len(data)), contentType)
}

// PutReader tải object theo luồng, phù hợp với file lớn.
func (s *Store) PutReader(
	ctx context.Context,
	key string,
	reader io.Reader,
	size int64,
	contentType string,
) (port.StoredObject, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, reader, size,
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return port.StoredObject{}, fmt.Errorf("put object %q thất bại: %w", key, err)
	}
	return port.StoredObject{Key: key, Size: info.Size, ContentType: contentType}, nil
}

// Get tải object về bộ nhớ.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.GetReader(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = obj.Close() }()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("đọc object %q thất bại: %w", key, err)
	}
	return data, nil
}

// GetReader mở object dưới dạng stream; caller phải đóng reader.
func (s *Store) GetReader(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q thất bại: %w", key, err)
	}
	return obj, nil
}

// PresignedGetURL tạo URL tải object có thời hạn.
func (s *Store) PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.presign.PresignedGetObject(ctx, s.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign get %q thất bại: %w", key, err)
	}
	return u.String(), nil
}

// PresignedPutURL tạo URL tải object LÊN có thời hạn — dùng cho luồng client
// tự upload thẳng lên storage, backend không đụng vào bytes file.
func (s *Store) PresignedPutURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.presign.PresignedPutObject(ctx, s.bucket, key, ttl)
	if err != nil {
		return "", fmt.Errorf("presign put %q thất bại: %w", key, err)
	}
	return u.String(), nil
}

// Stat kiểm tra object đã thực sự tồn tại trong bucket chưa — dùng để xác
// nhận client đã upload xong qua presigned URL. Không tồn tại -> port.ErrObjectNotFound.
func (s *Store) Stat(ctx context.Context, key string) (port.StoredObject, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if errResp := minio.ToErrorResponse(err); errResp.Code == "NoSuchKey" {
			return port.StoredObject{}, port.ErrObjectNotFound
		}
		return port.StoredObject{}, fmt.Errorf("stat object %q thất bại: %w", key, err)
	}
	return port.StoredObject{Key: key, Size: info.Size, ContentType: info.ContentType}, nil
}

// Delete xóa object.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("xóa object %q thất bại: %w", key, err)
	}
	return nil
}
