//go:build integration

// Package integration chứa các test tích hợp cấp hệ thống (chạy với build tag
// `integration`). Hiện có 1 smoke test xác nhận chuỗi cấu hình -> DSN hoạt động;
// các kịch bản end-to-end sâu hơn (repo + DB thật) nằm ở
// internal/module/user/repository/*_integration_test.go.
package integration

import (
	"testing"

	"github.com/stretchr/testify/require"

	"document-hub-api/internal/config"
)

func TestConfig_LoadLocalForIntegration(t *testing.T) {
	cfg, err := config.Load("../../configs/config.local.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, cfg.MySQL.DSN())
}
