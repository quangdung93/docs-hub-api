package bootstrap

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/config"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/ai/ragflow"
	rediscache "github.com/quangdung93/docs-hub-api/internal/infrastructure/cache/redis"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/mq"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/mq/rabbitmq"
	objectstorage "github.com/quangdung93/docs-hub-api/internal/infrastructure/storage"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/telemetry"
	"github.com/quangdung93/docs-hub-api/pkg/hashing"
	"github.com/quangdung93/docs-hub-api/pkg/jwt"
)

// Infra gom toàn bộ client hạ tầng và các port đã hiện thực hóa.
type Infra struct {
	Log     *zap.Logger
	Metrics *telemetry.Metrics
	Tracer  *telemetry.TracerProvider

	DB     *gorm.DB
	Redis  *goredis.Client
	MQConn *rabbitmq.Connection // nil nếu RabbitMQ tắt

	Tx          port.TxManager
	Cache       port.Cache
	Publisher   port.Publisher
	ObjectStore port.ObjectStore
	RAG         port.RAGClient
	Hasher      *hashing.Hasher
	JWT         *jwt.Manager

	Checkers []port.HealthChecker
}

// NewInfra khởi tạo mọi client hạ tầng theo config. Lỗi ở bất kỳ dependency bắt
// buộc nào (DB, Redis) sẽ dừng khởi động (fail-fast).
func NewInfra(ctx context.Context, cfg *config.Config, log *zap.Logger, metrics *telemetry.Metrics, tracer *telemetry.TracerProvider) (*Infra, error) {
	infra := &Infra{Log: log, Metrics: metrics, Tracer: tracer}

	if err := infra.initPostgres(cfg, log); err != nil {
		return nil, err
	}
	if err := infra.initRedis(ctx, cfg); err != nil {
		return nil, err
	}
	if err := infra.initRabbitMQ(cfg); err != nil {
		return nil, err
	}
	if err := infra.initObjectStore(ctx, cfg); err != nil {
		return nil, err
	}
	infra.initRAGFlow(cfg)
	if err := infra.initSecurity(cfg); err != nil {
		return nil, err
	}
	return infra, nil
}

func (i *Infra) initRAGFlow(cfg *config.Config) {
	if !cfg.RAGFlow.Enabled {
		i.Log.Info("RAGFlow đang tắt")
		return
	}
	client := ragflow.New(cfg.RAGFlow.BaseURL, cfg.RAGFlow.APIKey, cfg.RAGFlow.Timeout, cfg.RAGFlow.UploadTimeout)
	i.RAG = client
	i.Checkers = append(i.Checkers, ragflow.NewHealthChecker(client))
	i.Log.Info("đã khởi tạo RAGFlow client", zap.String("base_url", cfg.RAGFlow.BaseURL))
}

func (i *Infra) initPostgres(cfg *config.Config, log *zap.Logger) error {
	db, err := postgres.New(postgres.Config{
		DSN:             cfg.Postgres.DSN(),
		MaxOpenConns:    cfg.Postgres.MaxOpenConns,
		MaxIdleConns:    cfg.Postgres.MaxIdleConns,
		ConnMaxLifetime: cfg.Postgres.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Postgres.ConnMaxIdleTime,
	}, log)
	if err != nil {
		return fmt.Errorf("khởi tạo PostgreSQL: %w", err)
	}
	i.DB = db
	i.Tx = postgres.NewTxManager(db)
	i.Checkers = append(i.Checkers, postgres.NewHealthChecker(db))
	return nil
}

func (i *Infra) initRedis(ctx context.Context, cfg *config.Config) error {
	client, err := rediscache.New(ctx, rediscache.Config{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		PoolSize: cfg.Redis.PoolSize,
	})
	if err != nil {
		return fmt.Errorf("khởi tạo Redis: %w", err)
	}
	i.Redis = client
	i.Cache = rediscache.NewCache(client)
	i.Checkers = append(i.Checkers, rediscache.NewHealthChecker(client))
	return nil
}

func (i *Infra) initRabbitMQ(cfg *config.Config) error {
	if !cfg.RabbitMQ.Enabled {
		i.Publisher = mq.NoopPublisher{}
		i.Log.Warn("RabbitMQ đang TẮT — dùng NoopPublisher")
		return nil
	}
	conn, err := rabbitmq.New(rabbitmq.Config{URL: cfg.RabbitMQ.URL(), Exchange: cfg.RabbitMQ.Exchange})
	if err != nil {
		return fmt.Errorf("khởi tạo RabbitMQ: %w", err)
	}
	i.MQConn = conn
	i.Publisher = rabbitmq.NewPublisher(conn)
	i.Checkers = append(i.Checkers, rabbitmq.NewHealthChecker(conn))
	return nil
}

func (i *Infra) initObjectStore(ctx context.Context, cfg *config.Config) error {
	store, checker, err := objectstorage.New(ctx, cfg)
	if err != nil {
		return err
	}
	i.ObjectStore = store
	i.Checkers = append(i.Checkers, checker)
	i.Log.Info("đã khởi tạo object storage", zap.String("driver", cfg.Storage.Driver))
	return nil
}

// clock trả về đồng hồ hệ thống (dùng cho các module).
func (i *Infra) clock() port.Clock { return port.SystemClock{} }

func (i *Infra) initSecurity(cfg *config.Config) error {
	i.Hasher = hashing.NewHasher(0) // bcrypt default cost

	mgr, err := jwt.NewManager(jwt.Config{
		Algorithm:     cfg.JWT.Algorithm,
		Secret:        cfg.JWT.Secret,
		PrivateKeyPEM: cfg.JWT.PrivateKey,
		Issuer:        cfg.JWT.Issuer,
		AccessTTL:     cfg.JWT.AccessTTL,
	})
	if err != nil {
		return fmt.Errorf("khởi tạo JWT manager: %w", err)
	}
	i.JWT = mgr
	return nil
}

// Close đóng các client theo THỨ TỰ NGƯỢC với khởi tạo (MQ -> Redis -> DB).
// Tracer được đóng riêng ở app.go sau cùng.
func (i *Infra) Close(_ context.Context) {
	if i.MQConn != nil {
		if err := i.MQConn.Close(); err != nil {
			i.Log.Error("đóng RabbitMQ lỗi", zap.Error(err))
		}
	}
	if i.Redis != nil {
		if err := i.Redis.Close(); err != nil {
			i.Log.Error("đóng Redis lỗi", zap.Error(err))
		}
	}
	if i.DB != nil {
		if err := postgres.Close(i.DB); err != nil {
			i.Log.Error("đóng PostgreSQL lỗi", zap.Error(err))
		}
	}
}
