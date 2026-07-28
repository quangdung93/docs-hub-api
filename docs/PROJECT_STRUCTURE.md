# Cấu trúc thư mục — docs-hub-api

Tài liệu này giải thích **vì sao mỗi thư mục tồn tại** và ranh giới phụ thuộc giữa chúng. Đọc kèm `docs/architecture/ADR-0001` (vertical slice) và `ADR-0002` (tách lỗi).

## Sơ đồ tổng quan

```
docs-hub-api/
├── cmd/                  # Điểm vào các chương trình (main package)
│   ├── api/              # HTTP API server
│   ├── migrate/          # Chạy DB migration (bước riêng, không nằm trong api)
│   └── seed/             # Nạp dữ liệu mẫu (chặn production)
├── internal/             # Code riêng của service — không import được từ ngoài module
│   ├── bootstrap/        # Composition root: ráp mọi thứ + graceful shutdown
│   ├── config/           # Struct config + loader Viper (YAML + ENV)
│   ├── common/           # Shared kernel — dùng chung ≥2 module
│   │   ├── errcode/      # Hằng số mã lỗi ISC + map mã→HTTP
│   │   ├── apperr/       # BusinessError / TechnicalError
│   │   ├── response/     # Envelope ISC {success,data,error,meta} — nơi DUY NHẤT ghi JSON
│   │   ├── pagination/   # Chuẩn hóa phân trang (query + meta)
│   │   ├── contextx/     # request_id, trace_id, actor trong context
│   │   ├── validatorx/   # Bọc validator: tên field snake_case, đổi lỗi→details
│   │   └── port/         # Interface hạ tầng dùng chung (TxManager, Cache, Publisher, ObjectStore, Clock, HealthChecker)
│   ├── infrastructure/   # Nơi DUY NHẤT biết gorm/redis/amqp/minio
│   │   ├── database/mysql/   # GORM + transaction/context + errmap + health
│   │   ├── cache/redis/      # Client + port.Cache + health
│   │   ├── mq/               # NoopPublisher + rabbitmq/ (publisher, health)
│   │   ├── storage/minio/    # port.ObjectStore + health
│   │   ├── telemetry/        # OTel tracer + Prometheus metrics (registry riêng)
│   │   └── httpserver/       # Gin engine builder + API server + Admin server
│   ├── middleware/       # Chuỗi middleware HTTP (nơi khác delivery duy nhất import gin)
│   └── module/           # Các vertical slice (feature)
│       ├── health/       # Liveness + readiness (checker song song)
│       ├── user/         # MODULE MẪU — hiện thực đầy đủ
│       │   ├── domain/       # Entity, rule, port repository, lỗi nghiệp vụ — THUẦN (không gin/gorm)
│       │   ├── usecase/      # Service: điều phối, transaction, cache, publish — THUẦN
│       │   ├── repository/   # GORM model + mapper + scopes + impl port
│       │   ├── delivery/http/# Handler (mỏng) + dto + presenter + route
│       │   ├── mocks/        # Mock sinh bởi mockery
│       │   └── module.go     # Ráp repo→service→handler của feature
│       └── auth,file,notification,tenant/  # SCAFFOLD (README + doc.go)
├── pkg/                  # Thư viện THUẦN, tái sử dụng ngoài repo (không import internal/)
│   ├── logger/           # Zap builder + context helper
│   ├── jwt/              # Ký/verify JWT
│   ├── hashing/          # bcrypt
│   └── ptr/              # Tiện ích con trỏ generic
├── migrations/           # SQL versioned cho golang-migrate ({v}_{name}.{up|down}.sql)
├── configs/              # config.{local,dev,staging,production}.yaml (KHÔNG secret thật)
├── deployments/
│   ├── docker/           # Dockerfile (multi-stage, distroless) + .dockerignore
│   └── compose/          # docker-compose + otel-collector + prometheus
├── docs/
│   ├── api/openapi/      # Đặc tả OpenAPI 3.0 (bắt buộc theo templates/07)
│   ├── architecture/     # ADR-0001..0005
│   ├── swagger/          # docs.go sinh bởi swaggo (`make swagger`)
│   └── PROJECT_STRUCTURE.md  # (tài liệu này)
├── test/integration/     # Test tích hợp cấp hệ thống (build tag integration)
└── scripts/              # githooks/pre-commit, rename-module.sh
```

## Ranh giới phụ thuộc (bắt buộc)

```
cmd ──→ bootstrap ──→ (config, infrastructure, middleware, module/*)
                            │
module/<feature>:  delivery ──→ usecase ──→ domain ←── repository
                                              ▲
                        (chỉ stdlib + common/apperr + common/pagination + common/port)

domain, usecase   ⟹  KHÔNG import gin / gorm / redis / amqp / minio   (depguard chặn)
infrastructure    ⟹  nơi DUY NHẤT import các thư viện hạ tầng
pkg/*             ⟹  KHÔNG import internal/*  (giữ thuần để tái sử dụng)
```

## Vì sao bố cục thế này (tóm tắt)
- **Slice-first**: mỗi feature tự trị, mỗi tầng là 1 package Go thật → depguard enforce được "business không phụ thuộc framework" ở mức build. (ADR-0001)
- **common/ là kernel tối thiểu**: chỉ đặt thứ ≥2 module dùng chung, tránh phình thành god-package.
- **infrastructure/ cô lập thư viện ngoài**: đổi Redis→Valkey, RabbitMQ→Kafka chỉ đụng 1 thư mục.
- **cmd/ tách 3 binary**: migrate/seed là thao tác vận hành riêng, không lẫn vào vòng đời API.

## Thêm feature mới
1. `cp -r internal/module/user internal/module/<new>` rồi sửa nội dung.
2. Thêm migration vào `migrations/`.
3. Thêm 1 dòng vào `internal/bootstrap/modules.go`.
Xem thêm `CLAUDE.md`.
