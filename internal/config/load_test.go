package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/config"
)

// localConfigPath trả về đường dẫn config.local.yaml tính từ thư mục package.
func localConfigPath(t *testing.T) string {
	t.Helper()
	// internal/config -> lên 2 cấp tới gốc repo -> configs/
	return filepath.Join("..", "..", "configs", "config.local.yaml")
}

func TestLoad_LocalConfig(t *testing.T) {
	cfg, err := config.Load(localConfigPath(t))
	require.NoError(t, err)
	require.Equal(t, "docs-hub-api", cfg.App.Name)
	require.Equal(t, config.EnvLocal, cfg.App.Env)
	require.True(t, cfg.App.IsLocal())
	require.Equal(t, 8080, cfg.HTTP.APIPort)
	require.Equal(t, 9090, cfg.HTTP.AdminPort)
	require.Equal(t, "filesystem", cfg.Storage.Driver)
	require.Equal(t, "./var/storage", cfg.Storage.Filesystem.Root)
}

// TestLoad_EnvOverridesYAML là bằng chứng ENV override được YAML.
func TestLoad_EnvOverridesYAML(t *testing.T) {
	t.Setenv("APP_POSTGRES_PASSWORD", "secret-from-env")
	t.Setenv("APP_JWT_SECRET", "jwt-from-env")
	t.Setenv("APP_HTTP_API_PORT", "9999")

	cfg, err := config.Load(localConfigPath(t))
	require.NoError(t, err)

	require.Equal(t, "secret-from-env", cfg.Postgres.Password, "ENV phải override password trong YAML")
	require.Equal(t, "jwt-from-env", cfg.JWT.Secret)
	require.Equal(t, 9999, cfg.HTTP.APIPort, "ENV phải override cả kiểu số")
}

// TestLoad_RejectsDevTokenOutsideLocal chứng minh checkSafety chặn cấu hình nguy hiểm.
func TestLoad_RejectsDevTokenOutsideLocal(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_APP_ENV", "production") // app.env
	t.Setenv("APP_APP_ENABLE_DEV_TOKEN", "true")
	t.Setenv("APP_JWT_SECRET", "x")

	_, err := config.Load(localConfigPath(t))
	require.Error(t, err)
	require.Contains(t, err.Error(), "enable_dev_token")
}

func TestPostgresConfig_DSN(t *testing.T) {
	p := config.PostgresConfig{
		Host: "127.0.0.1", Port: 5432, User: "app", Password: "p",
		Database: "document_hub", SSLMode: "disable",
	}
	require.Equal(t,
		"host=127.0.0.1 port=5432 user=app password=p dbname=document_hub sslmode=disable TimeZone=UTC",
		p.DSN(),
	)
	require.Equal(t,
		"postgres://app:p@127.0.0.1:5432/document_hub?sslmode=disable",
		p.MigrationDSN(),
	)
}
