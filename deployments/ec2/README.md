# Chạy docs-hub-api trên một máy EC2

Toàn bộ stack (API + Postgres + Redis + RabbitMQ + MinIO) chạy bằng docker compose
ngay trên instance, không phụ thuộc dịch vụ AWS nào khác.

## 1. Yêu cầu instance

| Hạng mục | Khuyến nghị | Ghi chú |
|---|---|---|
| Loại máy | `t3.medium` (2 vCPU / 4GB) trở lên | build Go trong container ngốn RAM; máy nhỏ hơn cần swap (script tự tạo) |
| Ổ đĩa | ≥ 30GB gp3 | image + volume Postgres/MinIO |
| OS | Ubuntu 22.04+ hoặc Amazon Linux 2023 | `bootstrap.sh` hỗ trợ cả hai |

Security group — mở tối thiểu:

| Port | Nguồn | Dùng cho |
|---|---|---|
| 22 | IP của bạn | SSH |
| 8080 | IP của bạn / ALB | API + Swagger |

Các cổng còn lại (5432, 6379, 5672, 15672, 9000, 9001, 9090) **cố tình chỉ bind
`127.0.0.1`** trong compose. Cần xem thì dùng SSH tunnel, ví dụ RabbitMQ UI:

```bash
ssh -i key.pem -L 15672:127.0.0.1:15672 ubuntu@<ip>
```

## 2. Cài đặt (chạy một lần)

```bash
cd /path/den/docs-hub-api
bash deployments/ec2/bootstrap.sh
# nếu script báo cần logout: thoát SSH rồi vào lại
```

Script cài Docker + compose plugin, thêm user vào group `docker`, và tạo swapfile
4GB nếu máy dưới 4GB RAM.

## 3. Cấu hình secret

```bash
cp .env.ec2.example .env.ec2
vi .env.ec2
```

Bắt buộc điền: `APP_POSTGRES_PASSWORD`, `APP_RABBITMQ_PASSWORD`,
`APP_MINIO_SECRET_KEY` (≥8 ký tự), `APP_JWT_SECRET`. Thiếu biến nào compose sẽ
báo đúng tên biến đó và dừng trước khi build.

Sinh secret: `openssl rand -base64 32`.

## 4. Chạy

```bash
make ec2-up      # build image + chạy migration + khởi động (lần đầu ~3-5 phút)
make ec2-ps
make ec2-logs
```

Kiểm tra:

```bash
curl -s localhost:9090/healthz     # liveness
curl -s localhost:9090/readyz      # dependency đã sẵn sàng chưa
curl -s localhost:9090/metrics     # prometheus
```

Route nghiệp vụ nằm dưới `/public/api/v1/...` (không cần JWT) và
`/internal/api/v1/...` (cần JWT) — xem `internal/bootstrap/router.go`.

Từ máy bạn: `http://<ip-ec2>:8080`, Swagger ở
`http://<ip-ec2>:8080/swagger/index.html`.

## 5. Cập nhật code

```bash
git pull
make ec2-restart   # build lại api + chạy migration mới, hạ tầng giữ nguyên
```

## Lưu ý khác biệt so với local

- **`configs/config.ec2.yaml`** dùng `app.env: dev` (loader chỉ nhận
  `local|dev|staging|production`), tracing tắt, pprof tắt, swagger bật.
- **Secret bắt buộc từ ENV**: ngoài `env=local`, loader chặn khởi động nếu thiếu
  `jwt.secret` (xem `internal/config/load.go`).
- **`enable_dev_token` không bật được** ngoài local — loader chặn.
- **CORS** mặc định chỉ cho `http://localhost:3000`. Có frontend thật thì thêm
  domain vào `cors.allowed_origins` rồi `make ec2-restart`.
- Muốn HTTPS/domain: đặt ALB hoặc nginx + certbot trước cổng 8080, rồi đóng 8080
  khỏi internet và chỉ cho ALB/nginx truy cập.
