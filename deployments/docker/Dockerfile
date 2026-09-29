# syntax=docker/dockerfile:1
# Multi-stage build: builder (golang) -> runtime Debian tối giản, nonroot.
# Runtime cần poppler-utils để worker trích xuất PDF có text layer.

# ---------- Stage build ----------
FROM golang:1.25 AS builder

WORKDIR /src

# Ghi đè khi máy build không tới được proxy.golang.org (VPS CloudZ bị chặn
# storage.googleapis.com). Proxy khác vẫn an toàn vì go.sum kiểm tra checksum.
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

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

# Mirror apt thay cho deb.debian.org, ví dụ http://mirror.bizflycloud.vn/debian.
# Để trống = giữ nguồn mặc định. Chỉ dùng khi máy build không ra được
# deb.debian.org (VPS CloudZ); khi đặt, repo debian-security bị bỏ vì mirror
# trong nước thường không có.
ARG APT_MIRROR=""
RUN if [ -n "$APT_MIRROR" ]; then \
    printf 'Types: deb\nURIs: %s\nSuites: bookworm bookworm-updates\nComponents: main\nSigned-By: /usr/share/keyrings/debian-archive-keyring.gpg\n' \
    "$APT_MIRROR" > /etc/apt/sources.list.d/debian.sources; \
    fi

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

# Runtime chạy bằng non-root; tạo trước các thư mục mà config dùng đường dẫn tương đối.
RUN mkdir -p /app/logs /app/var/storage && chown -R app:app /app/logs /app/var

USER app

EXPOSE 8080 9090

ENTRYPOINT ["/app/api"]
