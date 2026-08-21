# Makefile — docs-hub-api
# Mục tiêu: mọi thao tác thường dùng chỉ 1 lệnh, chạy được cả local lẫn CI.

APP_NAME       := docs-hub-api
BIN_DIR        := bin
MAIN_API       := ./cmd/api
MAIN_MIGRATE   := ./cmd/migrate
MAIN_SEED      := ./cmd/seed
MAIN_WORKER    := ./cmd/worker
CONFIG         ?= configs/config.local.yaml
COMPOSE_FILE   := deployments/compose/docker-compose.yml
GIT_SHA        := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS        := -s -w -X main.version=$(GIT_SHA)

# Công cụ (pin version để mọi máy giống nhau).
# LƯU Ý: golangci-lint phải là nhánh v2 — bản v1.61 KHÔNG build được với
# Go 1.25 (lỗi trong golang.org/x/tools), và .golangci.yml đã là schema v2.
# Module path của v2 có thêm "/v2/". mockery v2.46 vướng đúng lỗi đó.
GOLANGCI_VERSION := v2.12.2
MOCKERY_VERSION  := v2.53.6
SWAG_VERSION     := v1.16.4
MIGRATE_VERSION  := v4.18.1

.DEFAULT_GOAL := help

.PHONY: help
help: ## Hiển thị danh sách lệnh
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

## ------------------------------------------------------------------ Build
.PHONY: build
build: ## Build cả 3 binary vào bin/
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/api $(MAIN_API)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/migrate $(MAIN_MIGRATE)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/seed $(MAIN_SEED)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/worker $(MAIN_WORKER)

.PHONY: run-worker
run-worker: ## Chạy ingestion worker local
	APP_ENV=local go run $(MAIN_WORKER) -config $(CONFIG)

.PHONY: run
run: ## Chạy API ở môi trường local
	APP_ENV=local go run $(MAIN_API) -config $(CONFIG)

.PHONY: tidy
tidy: ## go mod tidy + verify
	go mod tidy
	go mod verify

## ------------------------------------------------------------------ Quality
.PHONY: fmt
fmt: ## Format code (gofmt + goimports)
	gofmt -w .
	go run golang.org/x/tools/cmd/goimports@latest -w -local $(APP_NAME) .

.PHONY: vet
vet: ## go vet
	go vet ./...

.PHONY: lint
lint: ## Chạy golangci-lint trên TOÀN BỘ repo (gồm cả nợ cũ trên main)
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run --timeout 5m

.PHONY: lint-new
lint-new: ## Chỉ báo lỗi MỚI so với main — đúng thứ CI chặn merge
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run \
		--timeout 5m --new-from-rev=origin/main

.PHONY: lint-fmt-check
lint-fmt-check: ## Kiểm tra code đã format chưa (dùng trong CI)
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || (echo "❌ Code chưa gofmt, chạy 'make fmt'"; exit 1)

## ------------------------------------------------------------------ Test
.PHONY: test
test: ## Unit test (nhanh, không cần Docker)
	go test -race -short -coverprofile=coverage.out ./...

.PHONY: cover
cover: test ## Báo cáo coverage dạng HTML
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | tail -1

.PHONY: test-integration
test-integration: ## Integration test (cần Docker daemon)
	go test -tags=integration -race -v ./internal/module/user/repository/... ./test/integration/...

## ------------------------------------------------------------------ Codegen
.PHONY: mocks
mocks: ## Sinh mock bằng mockery
	go run github.com/vektra/mockery/v2@$(MOCKERY_VERSION)

.PHONY: swagger
swagger: ## Sinh tài liệu Swagger từ annotation
	go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) init \
		-g cmd/api/main.go -o docs/swagger --parseInternal --parseDepth 2

## ------------------------------------------------------------------ Database
.PHONY: migrate-up
migrate-up: ## Chạy migration lên mới nhất
	go run $(MAIN_MIGRATE) -config $(CONFIG) up

.PHONY: migrate-down
migrate-down: ## Rollback 1 bước migration
	go run $(MAIN_MIGRATE) -config $(CONFIG) down 1

.PHONY: migrate-create
migrate-create: ## Tạo migration mới: make migrate-create name=create_xxx
	@test -n "$(name)" || (echo "Thiếu name: make migrate-create name=create_xxx"; exit 1)
	go run github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION) \
		create -ext sql -dir migrations -seq $(name)

.PHONY: seed
seed: ## Nạp dữ liệu mẫu
	go run $(MAIN_SEED) -config $(CONFIG)

## ------------------------------------------------------------------ Infra
.PHONY: up
up: ## Bật toàn bộ hạ tầng local (docker compose)
	docker compose -f $(COMPOSE_FILE) up -d

.PHONY: up-local
up-local: ## Bật dependency tối thiểu cho API local dùng filesystem storage
	docker compose -f $(COMPOSE_FILE) up -d postgres redis rabbitmq

.PHONY: up-ragflow
up-ragflow: ## Bật stack kèm ingestion worker RAGFlow (cần APP_RAGFLOW_* trong .env)
	docker compose -f $(COMPOSE_FILE) --profile ragflow up -d

.PHONY: down
down: ## Tắt hạ tầng local
	docker compose -f $(COMPOSE_FILE) down

.PHONY: logs
logs: ## Xem log hạ tầng
	docker compose -f $(COMPOSE_FILE) logs -f

.PHONY: ps
ps: ## Trạng thái hạ tầng
	docker compose -f $(COMPOSE_FILE) ps

## ------------------------------------------------------------------ EC2
# Stack all-in-one chạy trực tiếp trên máy EC2. Secret nằm ở .env.ec2 (không commit).
EC2_COMPOSE := deployments/ec2/docker-compose.yml
EC2_ENV     := .env.ec2

.PHONY: ec2-up
ec2-up: ## Build + chạy toàn bộ stack trên EC2
	@test -f $(EC2_ENV) || (echo "❌ Thiếu $(EC2_ENV) — cp .env.ec2.example $(EC2_ENV) rồi điền secret"; exit 1)
	docker compose -f $(EC2_COMPOSE) --env-file $(EC2_ENV) up -d --build

.PHONY: ec2-down
ec2-down: ## Tắt stack EC2 (giữ nguyên volume dữ liệu)
	docker compose -f $(EC2_COMPOSE) --env-file $(EC2_ENV) down

.PHONY: ec2-logs
ec2-logs: ## Xem log api trên EC2
	docker compose -f $(EC2_COMPOSE) --env-file $(EC2_ENV) logs -f api

.PHONY: ec2-ps
ec2-ps: ## Trạng thái stack EC2
	docker compose -f $(EC2_COMPOSE) --env-file $(EC2_ENV) ps

.PHONY: ec2-restart
ec2-restart: ## Build lại api sau khi pull code mới
	docker compose -f $(EC2_COMPOSE) --env-file $(EC2_ENV) up -d --build migrate api

## ------------------------------------------------------------------ Docker
.PHONY: docker-build
docker-build: ## Build Docker image
	docker build -f deployments/docker/Dockerfile -t $(APP_NAME):$(GIT_SHA) .

## ------------------------------------------------------------------ Hooks
.PHONY: hooks
hooks: ## Cài git pre-commit hook
	git config core.hooksPath scripts/githooks
	@echo "✅ Đã trỏ core.hooksPath -> scripts/githooks"

## ------------------------------------------------------------------ CI gộp
.PHONY: ci
ci: lint-fmt-check vet lint-new test ## Chạy toàn bộ kiểm tra như CI
