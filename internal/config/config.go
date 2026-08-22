// Package config định nghĩa toàn bộ cấu hình của service dưới dạng struct
// và nạp từ file YAML theo môi trường + biến môi trường (ENV override).
//
// Nguyên tắc:
//   - Không có biến global. Config được truyền qua constructor (DI).
//   - Secret (mật khẩu DB, JWT secret...) LUÔN đến từ ENV, không hardcode trong YAML.
//   - Mỗi trường có tag `validate` để fail-fast khi cấu hình sai.
package config

import "time"

// Config là gốc cấu hình, gom toàn bộ thành phần con.
type Config struct {
	App       AppConfig       `mapstructure:"app"       validate:"required"`
	HTTP      HTTPConfig      `mapstructure:"http"      validate:"required"`
	Log       LogConfig       `mapstructure:"log"       validate:"required"`
	Postgres  PostgresConfig  `mapstructure:"postgres"  validate:"required"`
	Redis     RedisConfig     `mapstructure:"redis"     validate:"required"`
	RabbitMQ  RabbitMQConfig  `mapstructure:"rabbitmq"`
	Storage   StorageConfig   `mapstructure:"storage" validate:"required"`
	MinIO     MinIOConfig     `mapstructure:"minio"`
	JWT       JWTConfig       `mapstructure:"jwt"       validate:"required"`
	CORS      CORSConfig      `mapstructure:"cors"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	Timeout   TimeoutConfig   `mapstructure:"timeout"   validate:"required"`
	LocalAI   LocalAIConfig   `mapstructure:"local_ai"`
	RAGFlow   RAGFlowConfig   `mapstructure:"ragflow"`
	Ingestion IngestionConfig `mapstructure:"ingestion"`
	Project   ProjectConfig   `mapstructure:"project" validate:"required"`
}

// StorageConfig chọn backend lưu tài liệu gốc.
type StorageConfig struct {
	Driver     string           `mapstructure:"driver" validate:"required,oneof=filesystem minio"`
	Filesystem FilesystemConfig `mapstructure:"filesystem"`
}

// FilesystemConfig cấu hình lưu file trong một thư mục local.
type FilesystemConfig struct {
	Root string `mapstructure:"root"`
}

// LocalAIConfig cấu hình các endpoint OpenAI-compatible của LocalAI.
type LocalAIConfig struct {
	BaseURL            string        `mapstructure:"base_url"`
	ChatModel          string        `mapstructure:"chat_model"`
	EmbeddingModel     string        `mapstructure:"embedding_model"`
	EmbeddingDimension int           `mapstructure:"embedding_dimension"`
	Timeout            time.Duration `mapstructure:"timeout"`
}

// RAGFlowConfig cấu hình RAGFlow HTTP API. APIKey luôn đến từ
// APP_RAGFLOW_API_KEY hoặc secret manager, không đặt trong YAML.
type RAGFlowConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	BaseURL         string        `mapstructure:"base_url"`
	APIKey          string        `mapstructure:"api_key"`
	DatasetPrefix   string        `mapstructure:"dataset_prefix"`
	Timeout         time.Duration `mapstructure:"timeout"`
	UploadTimeout   time.Duration `mapstructure:"upload_timeout"`
	PollInterval    time.Duration `mapstructure:"poll_interval"`
	MaxPollDuration time.Duration `mapstructure:"max_poll_duration"`
}

// IngestionConfig cấu hình worker polling và deterministic chunking.
type IngestionConfig struct {
	PollInterval time.Duration `mapstructure:"poll_interval"`
	ChunkLines   int           `mapstructure:"chunk_lines"`
	OverlapLines int           `mapstructure:"overlap_lines"`
	BatchSize    int           `mapstructure:"batch_size"`
}

// Environment là các môi trường được hỗ trợ.
type Environment string

const (
	EnvLocal      Environment = "local"
	EnvDev        Environment = "dev"
	EnvStaging    Environment = "staging"
	EnvProduction Environment = "production"
)

// AppConfig chứa thông tin định danh dịch vụ.
type AppConfig struct {
	Name string      `mapstructure:"name" validate:"required"`
	Env  Environment `mapstructure:"env"  validate:"required,oneof=local dev staging production"`
	// EnableDevToken bật endpoint /auth/dev-token để test.
	// Chỉ được phép true khi Env=local — loader sẽ chặn ở môi trường khác.
	EnableDevToken bool `mapstructure:"enable_dev_token"`
}

// IsLocal trả về true nếu đang chạy môi trường local.
func (a AppConfig) IsLocal() bool { return a.Env == EnvLocal }

// IsProduction trả về true nếu đang chạy production.
func (a AppConfig) IsProduction() bool { return a.Env == EnvProduction }

// HTTPConfig cấu hình 2 server: API (public) và Admin (nội bộ: metrics, health, pprof).
type HTTPConfig struct {
	APIPort         int           `mapstructure:"api_port"          validate:"required,min=1,max=65535"`
	AdminPort       int           `mapstructure:"admin_port"        validate:"required,min=1,max=65535"`
	MaxBodyBytes    int64         `mapstructure:"max_body_bytes"    validate:"required,min=1024"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"  validate:"required"`
	EnablePprof     bool          `mapstructure:"enable_pprof"`
	EnableSwagger   bool          `mapstructure:"enable_swagger"`
}

// LogConfig cấu hình logger Zap.
type LogConfig struct {
	Level    string `mapstructure:"level"    validate:"required,oneof=debug info warn error"`
	Encoding string `mapstructure:"encoding" validate:"required,oneof=json console"`
}

// PostgresConfig cấu hình kết nối PostgreSQL (có pgvector) qua GORM.
// Password đến từ ENV: APP_POSTGRES_PASSWORD.
type PostgresConfig struct {
	Host            string        `mapstructure:"host"     validate:"required"`
	Port            int           `mapstructure:"port"     validate:"required"`
	User            string        `mapstructure:"user"     validate:"required"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database" validate:"required"`
	SSLMode         string        `mapstructure:"ssl_mode" validate:"required,oneof=disable require verify-ca verify-full"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"    validate:"required,min=1"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"    validate:"required,min=1"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" validate:"required"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
}

// RedisConfig cấu hình Redis (cache + rate limit).
// Password đến từ ENV: APP_REDIS_PASSWORD.
type RedisConfig struct {
	Host     string `mapstructure:"host"     validate:"required"`
	Port     int    `mapstructure:"port"     validate:"required"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size" validate:"required,min=1"`
}

// RabbitMQConfig cấu hình message queue.
// Password đến từ ENV: APP_RABBITMQ_PASSWORD.
type RabbitMQConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	VHost    string `mapstructure:"vhost"`
	Exchange string `mapstructure:"exchange"`
}

// MinIOConfig cấu hình object storage (S3-compatible).
// SecretKey đến từ ENV: APP_MINIO_SECRET_KEY.
type MinIOConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
	UseSSL    bool   `mapstructure:"use_ssl"`
	Region    string `mapstructure:"region"`

	// PublicEndpoint là host mà TRÌNH DUYỆT gọi tới, ví dụ storage.docshub.io.vn.
	//
	// Chữ ký SigV4 của presigned URL bao gồm cả host, nên URL ký bằng endpoint
	// nội bộ ("minio:9000") không dùng được từ ngoài — và sửa host trong URL
	// bằng tay sẽ nhận SignatureDoesNotMatch. Đặt biến này để presigned URL
	// được ký sẵn theo host công khai; mọi thao tác khác vẫn đi đường nội bộ.
	//
	// Để trống ở local: khi đó presign dùng luôn Endpoint.
	PublicEndpoint string `mapstructure:"public_endpoint"`
	PublicUseSSL   bool   `mapstructure:"public_use_ssl"`
}

// ProjectConfig cấu hình nghiệp vụ ảnh đại diện dự án (module project).
// AvatarMaxBytes là giá trị TẠM THỜI (5 MiB), chờ team chốt lại chính thức.
type ProjectConfig struct {
	AvatarMaxBytes     int64         `mapstructure:"avatar_max_bytes"     validate:"required,min=1"`
	AvatarPresignedTTL time.Duration `mapstructure:"avatar_presigned_ttl" validate:"required"`
}

// JWTConfig cấu hình ký/verify token.
// Secret/khóa đến từ ENV: APP_JWT_SECRET.
type JWTConfig struct {
	Algorithm string `mapstructure:"algorithm"   validate:"required,oneof=HS256 RS256"`
	Secret    string `mapstructure:"secret"`
	// PrivateKey là khóa riêng RSA dạng PEM, BẮT BUỘC khi algorithm=RS256.
	// Là secret nên đặt qua ENV (APP_JWT_PRIVATE_KEY), không ghi vào file config.
	PrivateKey string        `mapstructure:"private_key"`
	Issuer     string        `mapstructure:"issuer"      validate:"required"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"  validate:"required"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl" validate:"required"`
}

// CORSConfig cấu hình CORS.
type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods"`
	AllowedHeaders   []string `mapstructure:"allowed_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"`
}

// RateLimitConfig cấu hình giới hạn số request (token bucket trên Redis).
type RateLimitConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	RequestsPerWindow int           `mapstructure:"requests_per_window"`
	Window            time.Duration `mapstructure:"window"`
}

// TelemetryConfig cấu hình OpenTelemetry tracing + Prometheus.
type TelemetryConfig struct {
	TracingEnabled bool    `mapstructure:"tracing_enabled"`
	OTLPEndpoint   string  `mapstructure:"otlp_endpoint"`
	SampleRatio    float64 `mapstructure:"sample_ratio"`
}

// TimeoutConfig gom các mốc timeout theo chuẩn ISC (templates/05).
// Không hardcode timeout trong code — luôn đọc từ đây.
type TimeoutConfig struct {
	ReadHeader time.Duration `mapstructure:"read_header" validate:"required"`
	Read       time.Duration `mapstructure:"read"        validate:"required"`
	Write      time.Duration `mapstructure:"write"       validate:"required"`
	Idle       time.Duration `mapstructure:"idle"        validate:"required"`
	Handler    time.Duration `mapstructure:"handler"     validate:"required"` // timeout xử lý 1 request
	DB         time.Duration `mapstructure:"db"          validate:"required"`
	Redis      time.Duration `mapstructure:"redis"       validate:"required"`
	MQ         time.Duration `mapstructure:"mq"`
	External   time.Duration `mapstructure:"external"`
}
