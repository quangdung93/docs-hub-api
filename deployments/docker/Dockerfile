# syntax=docker/dockerfile:1
# Multi-stage build: builder (golang) -> runtime Debian tối giản, nonroot.
# Runtime cần poppler-utils để worker trích xuất PDF có text layer.

# ---------- Stage build ----------
FROM golang:1.25 AS builder

WORKDIR /src

# Cache module: copy go.mod/go.sum trước để tận dụng layer cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
# CGO_ENABLED=0 để các binary Go không phụ thuộc thư viện động của runtime.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w" -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w" -o /out/seed ./cmd/seed && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w" -o /out/worker ./cmd/worker

# ---------- Stage runtime ----------
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates poppler-utils && \
    rm -rf /var/lib/apt/lists/* && \
    useradd --system --uid 65532 --no-create-home --shell /usr/sbin/nologin app

WORKDIR /app

# Binary + tài nguyên cần lúc chạy (config, migration SQL).
COPY --from=builder /out/api /app/api
COPY --from=builder /out/migrate /app/migrate
COPY --from=builder /out/seed /app/seed
COPY --from=builder /out/worker /app/worker
COPY --from=builder /src/configs /app/configs
COPY --from=builder /src/migrations /app/migrations

USER app

EXPOSE 8080 9090

ENTRYPOINT ["/app/api"]
