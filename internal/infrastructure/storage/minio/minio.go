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

	"document-hub-api/internal/common/port"
)

// Config là tham số kết nối MinIO.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	Region    string
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

// Store implement port.ObjectStore.
type Store struct {
	client *minio.Client
	bucket string
}

// NewStore bọc client thành port.ObjectStore.
func NewStore(client *minio.Client, bucket string) *Store {
	return &Store{client: client, bucket: bucket}
}

// Put tải object lên bucket.
func (s *Store) Put(ctx context.Context, key string, data []byte, contentType string) (port.StoredObject, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return port.StoredObject{}, fmt.Errorf("put object %q thất bại: %w", key, err)
	}
	return port.StoredObject{Key: key, Size: info.Size, ContentType: contentType}, nil
}

// Get tải object về bộ nhớ.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q thất bại: %w", key, err)
	}
	defer func() { _ = obj.Close() }()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("đọc object %q thất bại: %w", key, err)
	}
	return data, nil
}

// PresignedGetURL tạo URL tải object có thời hạn.
func (s *Store) PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign %q thất bại: %w", key, err)
	}
	return u.String(), nil
}

// Delete xóa object.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("xóa object %q thất bại: %w", key, err)
	}
	return nil
}
