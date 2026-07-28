package config

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// envPrefix là tiền tố cho biến môi trường override. Ví dụ:
//
//	APP_POSTGRES_PASSWORD -> postgres.password
//	APP_JWT_SECRET        -> jwt.secret
//	APP_HTTP_API_PORT   -> http.api_port
const envPrefix = "APP"

// Load nạp cấu hình theo thứ tự ưu tiên (thấp -> cao):
//  1. Giá trị mặc định (setDefaults)
//  2. File YAML theo môi trường (configPath)
//  3. Biến môi trường có tiền tố APP_
//
// Sau khi unmarshal, cấu hình được validate và kiểm tra ràng buộc an toàn
// (ví dụ: dev-token không được bật ngoài local).
func Load(configPath string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("đọc file cấu hình %q thất bại: %w", configPath, err)
		}
	}

	// ENV override: APP_POSTGRES_PASSWORD -> postgres.password
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal cấu hình thất bại: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	if err := checkSafety(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate chạy go-playground/validator trên toàn bộ struct config.
func validate(cfg *Config) error {
	val := validator.New(validator.WithRequiredStructEnabled())
	if err := val.Struct(cfg); err != nil {
		return fmt.Errorf("cấu hình không hợp lệ: %w", err)
	}
	return nil
}

// checkSafety chặn các cấu hình nguy hiểm không thể diễn đạt bằng tag validate.
func checkSafety(cfg *Config) error {
	// Không cho phép bật dev-token ngoài môi trường local — tránh lộ endpoint cấp token.
	if cfg.App.EnableDevToken && !cfg.App.IsLocal() {
		return fmt.Errorf(
			"cấu hình nguy hiểm: enable_dev_token=true nhưng env=%q (chỉ được bật ở local)",
			cfg.App.Env,
		)
	}

	// Ở môi trường thật, JWT HS256 bắt buộc phải có secret (thường từ ENV).
	if cfg.JWT.Algorithm == "HS256" && cfg.JWT.Secret == "" {
		return fmt.Errorf("thiếu jwt.secret (đặt qua ENV %s_JWT_SECRET)", envPrefix)
	}

	return nil
}

// setDefaults đặt giá trị mặc định an toàn cho các trường không bắt buộc,
// giúp file YAML gọn hơn và tránh lỗi zero-value.
func setDefaults(v *viper.Viper) {
	v.SetDefault("http.max_body_bytes", 1<<20) // 1 MiB
	v.SetDefault("http.shutdown_timeout", "15s")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.encoding", "json")

	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.ssl_mode", "disable")
	v.SetDefault("postgres.max_open_conns", 25)
	v.SetDefault("postgres.max_idle_conns", 10)
	v.SetDefault("postgres.conn_max_lifetime", "30m")
	v.SetDefault("postgres.conn_max_idle_time", "10m")

	v.SetDefault("redis.pool_size", 10)

	v.SetDefault("jwt.algorithm", "HS256")
	v.SetDefault("jwt.access_ttl", "15m")
	v.SetDefault("jwt.refresh_ttl", "168h")

	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.requests_per_window", 100)
	v.SetDefault("rate_limit.window", "1m")

	v.SetDefault("telemetry.sample_ratio", 1.0)

	v.SetDefault("timeout.read_header", "5s")
	v.SetDefault("timeout.read", "15s")
	v.SetDefault("timeout.write", "20s")
	v.SetDefault("timeout.idle", "60s")
	v.SetDefault("timeout.handler", "10s")
	v.SetDefault("timeout.db", "3s")
	v.SetDefault("timeout.redis", "2s")
	v.SetDefault("timeout.mq", "3s")
	v.SetDefault("timeout.external", "5s")
}
