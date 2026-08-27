package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
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
	// .env chỉ là nguồn tiện lợi khi chạy local. Biến môi trường đã có sẵn luôn
	// được giữ nguyên vì gotenv.Load không override process environment.
	if err := gotenv.Load(".env"); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("đọc file .env thất bại: %w", err)
	}

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

	// jwt.private_key KHÔNG có trong file yaml (là secret, không ghi vào repo),
	// mà AutomaticEnv chỉ áp dụng cho khóa viper đã biết — nên phải bind tay,
	// nếu không thì đặt APP_JWT_PRIVATE_KEY cũng vô tác dụng.
	//
	// Cố tình KHÔNG thêm khóa rỗng vào 5 file config: từng có sự cố production
	// vì config.ec2.yaml thiếu một khối bắt buộc mà không test nào chạm tới.
	if err := v.BindEnv("jwt.private_key"); err != nil {
		return nil, fmt.Errorf("bind ENV jwt.private_key: %w", err)
	}
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
	if cfg.JWT.Algorithm == "RS256" && strings.TrimSpace(cfg.JWT.PrivateKey) == "" {
		return fmt.Errorf("thiếu jwt.private_key cho RS256 (đặt qua ENV %s_JWT_PRIVATE_KEY)", envPrefix)
	}
	if cfg.Storage.Driver == "filesystem" && strings.TrimSpace(cfg.Storage.Filesystem.Root) == "" {
		return fmt.Errorf("thiếu storage.filesystem.root")
	}
	if cfg.Storage.Driver == "minio" && !cfg.MinIO.Enabled {
		return fmt.Errorf("storage.driver=minio yêu cầu minio.enabled=true")
	}
	if cfg.RAGFlow.Enabled {
		if strings.TrimSpace(cfg.RAGFlow.BaseURL) == "" {
			return fmt.Errorf("ragflow.enabled=true yêu cầu ragflow.base_url")
		}
		if strings.TrimSpace(cfg.RAGFlow.APIKey) == "" {
			return fmt.Errorf("ragflow.enabled=true yêu cầu API key qua ENV %s_RAGFLOW_API_KEY", envPrefix)
		}
		if strings.TrimSpace(cfg.RAGFlow.DatasetPrefix) == "" {
			return fmt.Errorf("ragflow.enabled=true yêu cầu ragflow.dataset_prefix")
		}
		validPrefix, _ := regexp.MatchString(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`, cfg.RAGFlow.DatasetPrefix)
		if !validPrefix {
			return fmt.Errorf("ragflow.dataset_prefix chỉ gồm chữ, số, _ hoặc - và dài tối đa 32 ký tự")
		}
		if cfg.RAGFlow.Timeout <= 0 || cfg.RAGFlow.UploadTimeout <= 0 ||
			cfg.RAGFlow.PollInterval <= 0 || cfg.RAGFlow.MaxPollDuration <= 0 {
			return fmt.Errorf("timeout/poll interval của ragflow phải lớn hơn 0")
		}
		if cfg.Timeout.Handler <= cfg.RAGFlow.Timeout {
			return fmt.Errorf("timeout.handler phải lớn hơn ragflow.timeout để request không bị hủy sớm")
		}
		if cfg.Timeout.Write <= cfg.Timeout.Handler {
			return fmt.Errorf("timeout.write phải lớn hơn timeout.handler khi bật ragflow")
		}
		if cfg.App.IsProduction() && !strings.HasPrefix(strings.ToLower(cfg.RAGFlow.BaseURL), "https://") {
			return fmt.Errorf("production yêu cầu ragflow.base_url dùng HTTPS")
		}
	}
	if cfg.MCP.Enabled && !cfg.RAGFlow.Enabled {
		return fmt.Errorf("mcp.enabled=true yêu cầu ragflow.enabled=true cho search/ask tools")
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
	v.SetDefault("storage.driver", "minio")
	v.SetDefault("storage.filesystem.root", "./var/storage")

	v.SetDefault("jwt.algorithm", "HS256")
	v.SetDefault("jwt.access_ttl", "15m")
	v.SetDefault("jwt.refresh_ttl", "168h")

	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.requests_per_window", 100)
	v.SetDefault("rate_limit.window", "1m")

	v.SetDefault("telemetry.sample_ratio", 1.0)
	v.SetDefault("local_ai.base_url", "http://127.0.0.1:8081")
	v.SetDefault("local_ai.chat_model", "")
	v.SetDefault("local_ai.embedding_model", "")
	v.SetDefault("local_ai.embedding_dimension", 0)
	v.SetDefault("local_ai.timeout", "30s")
	v.SetDefault("ragflow.enabled", false)
	v.SetDefault("ragflow.base_url", "http://127.0.0.1:9380")
	v.SetDefault("ragflow.api_key", "")
	v.SetDefault("ragflow.dataset_prefix", "docs_hub")
	v.SetDefault("ragflow.timeout", "30s")
	v.SetDefault("ragflow.upload_timeout", "2m")
	v.SetDefault("ragflow.poll_interval", "3s")
	v.SetDefault("ragflow.max_poll_duration", "15m")
	v.SetDefault("ingestion.poll_interval", "2s")
	v.SetDefault("ingestion.chunk_lines", 80)
	v.SetDefault("ingestion.overlap_lines", 10)
	v.SetDefault("ingestion.batch_size", 16)
	v.SetDefault("mcp.enabled", false)
	v.SetDefault("mcp.requests_per_window", 30)
	v.SetDefault("mcp.window", "1m")
	v.SetDefault("mcp.max_source_lines", 200)
	v.SetDefault("mcp.max_excerpt_chars", 65536)

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
