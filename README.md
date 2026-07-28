# docs-hub-api

Boilerplate backend Go 1.25 theo **Clean Architecture + DDD** — nền tảng cho các dịch vụ Go của ISC. Module `user` được hiện thực end-to-end làm bản mẫu; các module khác ở dạng scaffold.

> Đây là dịch vụ Go đầu tiên của ISC. Tuân theo chuẩn ISC (`templates/01..05,07`): envelope response, catalogue mã lỗi, quy ước đặt tên API, timeout.

## Công nghệ
Gin · GORM/PostgreSQL + pgvector · Redis · RabbitMQ · MinIO · Viper · Zap · validator · JWT · Swagger · OpenTelemetry · Prometheus · Docker.

## Kiến trúc (tóm tắt)
```
delivery ──→ usecase ──→ domain ←── repository
   (gin)     (thuần)     (thuần)     (gorm)
```
- Slice-first: mỗi feature là `internal/module/<name>/{domain,usecase,repository,delivery}`.
- `domain`/`usecase` KHÔNG import gin/gorm/redis — `golangci-lint depguard` chặn ở mức build.
- Chi tiết: `docs/architecture/ADR-0001..0005`.

## Bố cục thư mục
| Thư mục | Vai trò |
|---|---|
| `cmd/{api,migrate,seed}` | 3 binary. Migrate là bước riêng, không chạy lúc boot. |
| `internal/bootstrap` | Composition root (DI bằng constructor) + graceful shutdown. |
| `internal/config` | Struct config + loader Viper (YAML + ENV override). |
| `internal/common` | Shared kernel: `errcode`, `apperr`, `response`, `pagination`, `contextx`, `validatorx`, `port`. |
| `internal/infrastructure` | Nơi DUY NHẤT biết gorm/redis/amqp/minio + telemetry + httpserver. |
| `internal/middleware` | Chuỗi middleware HTTP (nơi duy nhất khác `delivery` import gin). |
| `internal/module/user` | Module mẫu hiện thực đầy đủ. |
| `internal/module/{auth,file,notification,tenant}` | Scaffold (README + doc.go). |
| `pkg/{logger,jwt,hashing,ptr}` | Thư viện thuần, tái sử dụng ngoài repo. |
| `migrations` | SQL versioned cho golang-migrate. |
| `configs` | `config.{local,dev,staging,production}.yaml` (KHÔNG chứa secret thật). |
| `deployments` | Dockerfile + docker-compose + otel/prometheus. |
| `docs` | ADR, chuẩn API, OpenAPI YAML, swagger sinh tự động. |

## Chạy nhanh (local)
```bash
# 0) Cần Go 1.25 (brew install go) + Docker.
make tidy                 # go mod tidy + verify (GATE kiểm tra version thư viện)
make up                   # bật PostgreSQL/Redis/RabbitMQ/MinIO/Jaeger/Prometheus
make migrate-up           # tạo schema
make seed                 # tạo admin@local / Admin@12345
make run                  # chạy API :8080, admin :9090
```

Kiểm tra:
```bash
curl -s localhost:9090/healthz          # {"status":"ok"}
curl -s localhost:9090/readyz | jq      # trạng thái từng dependency
curl -s localhost:9090/metrics | head   # Prometheus

# Lấy token dev (chỉ local)
TOKEN=$(curl -s -XPOST localhost:8080/public/api/v1/auth/dev-token \
  -H 'Content-Type: application/json' -d '{"email":"admin@local","roles":["admin"]}' | jq -r .data.access_token)

# Tạo user
curl -si -XPOST localhost:8080/internal/api/v1/users -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"email":"a@fpt.com","full_name":"Nguyễn Văn A","password":"P@ssw0rd123"}'
```

## Envelope chuẩn ISC
```json
{ "success": true, "data": {}, "error": null,
  "meta": { "request_id": "", "trace_id": "", "timestamp": "" } }
```
- Lỗi **nghiệp vụ** → HTTP 200 + `success:false` (ví dụ `DUPLICATE_EMAIL`).
- Lỗi **kỹ thuật** → 4xx/5xx (ví dụ `REQ_400`, `USR_404`, `SYS_500`).

## Test
```bash
make test               # unit (không cần Docker)
make test-integration   # repository + DB thật (testcontainers, cần Docker)
go test -run TestEnvelope ./internal/common/response/   # golden test chuẩn ISC
```

## Đổi module path (khi có URL git)
```bash
./scripts/rename-module.sh git.fpt.net/isc/document-hub/docs-hub-api
go build ./...
```

## Quan sát (observability)
- Health: `:9090/healthz` (liveness), `:9090/readyz` (readiness song song, timeout 2s).
- Metrics: `:9090/metrics` (registry riêng, không dùng global).
- Tracing: OpenTelemetry → Jaeger UI `:16686`. `trace_id` xuất hiện trong mọi response.
- Log: Zap có cấu trúc, kèm `request_id` + `trace_id`.

## Điểm cần biết
- **RabbitMQ/MinIO**: đã nối client + health check + port; module `user` chỉ dùng MQ để phát `user.created`. MinIO là điểm nối cho module `file`.
- **CI**: `.gitlab-ci.yml` tự chứa (không phụ thuộc template `isc/cicd-config` — chưa có bản Go). Cần xác nhận với đội nền tảng về template Go dài hạn.
- **Việc còn mở**: xem ADR-0003 (mã CONFLICT_VERSION chờ duyệt), ADR-0005 (Outbox cho publish sự kiện).
