package minio_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	miniostore "github.com/quangdung93/docs-hub-api/internal/infrastructure/storage/minio"
)

func baseConfig() miniostore.Config {
	return miniostore.Config{
		Endpoint:  "minio:9000",
		AccessKey: "app",
		SecretKey: "secret-du-dai-cho-test",
		Bucket:    "document-hub",
		Region:    "us-east-1",
	}
}

// Không cấu hình host công khai -> không dựng client riêng, Store ký bằng
// client nội bộ như trước.
func TestNewPresign_KhongCauHinh_TraNil(t *testing.T) {
	client, err := miniostore.NewPresign(baseConfig())
	require.NoError(t, err)
	require.Nil(t, client)
}

// Host công khai trùng host nội bộ thì cũng không cần client thứ hai.
func TestNewPresign_TrungEndpoint_TraNil(t *testing.T) {
	cfg := baseConfig()
	cfg.PublicEndpoint = cfg.Endpoint

	client, err := miniostore.NewPresign(cfg)
	require.NoError(t, err)
	require.Nil(t, client)
}

// Có host công khai -> dựng được client mà không cần chạm mạng (hàm chỉ tạo
// struct), nên gọi được cả khi DNS chưa trỏ.
func TestNewPresign_CoHostCongKhai_TraClient(t *testing.T) {
	cfg := baseConfig()
	cfg.PublicEndpoint = "storage.docshub.io.vn"
	cfg.PublicUseSSL = true

	client, err := miniostore.NewPresign(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "storage.docshub.io.vn", client.EndpointURL().Host)
	require.Equal(t, "https", client.EndpointURL().Scheme)
}

func TestNewPresign_EndpointSai_TraLoi(t *testing.T) {
	cfg := baseConfig()
	cfg.PublicEndpoint = "http://storage.docshub.io.vn" // endpoint phải là host, không kèm scheme

	_, err := miniostore.NewPresign(cfg)
	require.Error(t, err)
}
