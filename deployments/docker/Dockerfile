# syntax=docker/dockerfile:1
# Multi-stage build: builder (golang) -> runtime (distroless, nonroot).
# Kết quả: image nhỏ, không shell, chạy user không đặc quyền.

# ---------- Stage build ----------
FROM golang:1.25 AS builder

WORKDIR /src

# Cache module: copy go.mod/go.sum trước để tận dụng layer cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
# CGO_ENABLED=0 để build static binary chạy được trên distroless/static.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w" -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w" -o /out/seed ./cmd/seed

# ---------- Stage runtime ----------
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

# Binary + tài nguyên cần lúc chạy (config, migration SQL).
COPY --from=builder /out/api /app/api
COPY --from=builder /out/migrate /app/migrate
COPY --from=builder /out/seed /app/seed
COPY --from=builder /src/configs /app/configs
COPY --from=builder /src/migrations /app/migrations

# nonroot (uid 65532) đã được distroless đặt sẵn.
USER nonroot:nonroot

EXPOSE 8080 9090

ENTRYPOINT ["/app/api"]
