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
| 80 | Anywhere | ACME HTTP-01 của certbot + redirect sang HTTPS |
| 443 | Anywhere | nginx → API |

**Không mở 8080.** API bind `127.0.0.1:8080`, chỉ nginx trên cùng máy gọi được —
mọi request từ ngoài đều phải đi qua TLS. Cấu hình nginx nằm ở
`deployments/ec2/nginx/api.docshub.io.vn.conf`, cách cài đặt và cấp cert ghi
trong đầu file đó.

Các cổng còn lại (5432, 6379, 5672, 15672, 9000, 9001, 9090) cũng chỉ bind
`127.0.0.1`. Cần xem thì dùng SSH tunnel, ví dụ RabbitMQ UI:

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

### Deploy bằng GitHub Actions

Vào repository **Settings → Secrets and variables → Actions** và tạo các
Repository Secrets sau:

- `APP_POSTGRES_PASSWORD`
- `APP_REDIS_PASSWORD` (có thể để trống nếu Redis không dùng password)
- `APP_RABBITMQ_PASSWORD`
- `APP_MINIO_SECRET_KEY`
- `APP_JWT_SECRET`
- `APP_RAGFLOW_API_KEY`

Pipeline truyền trực tiếp các secret này cho Docker Compose; không đọc hay tạo
`.env.ec2` trên runner. Bước kiểm tra đầu deploy sẽ báo chính xác secret nào
đang thiếu trước khi pull image.

`APP_REDIS_PASSWORD` không bắt buộc vì Redis trong `docker-compose.yml` chạy
không `requirepass`; chỉ điền khi nào bật password cho Redis.

> **Quan trọng khi EC2 đã từng chạy stack thủ công.** Volume `postgres_data`
> chỉ nhận `POSTGRES_PASSWORD` ở lần init đầu tiên, và `pg_isready` không xác
> thực nên healthcheck vẫn xanh dù password sai. Vì vậy `APP_POSTGRES_PASSWORD`
> trên GitHub phải **trùng đúng** giá trị đã dùng lần đầu — lấy lại từ
> `.env.ec2` trên máy EC2, đừng sinh mới. Bước "Kiểm tra đăng nhập Postgres"
> trong pipeline sẽ dừng sớm và báo rõ nếu hai giá trị lệch nhau. Muốn đổi
> password thật sự thì `ALTER USER app PASSWORD '<mới>'` trong Postgres (hoặc
> xoá volume, chấp nhận mất dữ liệu) rồi mới cập nhật secret.

Đặt secret bằng CLI:

```bash
gh secret set APP_POSTGRES_PASSWORD --repo <owner>/<repo>
# lặp lại cho APP_RABBITMQ_PASSWORD, APP_MINIO_SECRET_KEY,
# APP_JWT_SECRET, APP_RAGFLOW_API_KEY
```

### Chạy/deploy thủ công trên EC2

```bash
cp .env.ec2.example .env.ec2
vi .env.ec2
```

Bắt buộc điền: `APP_POSTGRES_PASSWORD`, `APP_RABBITMQ_PASSWORD`,
`APP_MINIO_SECRET_KEY` (≥8 ký tự), `APP_JWT_SECRET` và
`APP_RAGFLOW_API_KEY`. Thiếu biến nào compose sẽ báo đúng tên biến đó và dừng
trước khi build. `APP_RAGFLOW_BASE_URL` mặc định là `https://ragflow.io.vn`;
đổi lại trong `.env.ec2` nếu production dùng instance khác.

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

Từ ngoài (qua nginx + TLS): `https://api.docshub.io.vn`, Swagger ở
`https://api.docshub.io.vn/swagger/index.html`.

## 5. Cập nhật code

Bình thường **không cần làm tay** — merge vào `main` là `.github/workflows/deploy.yml`
tự build image, đẩy lên GHCR và deploy (xem mục 6).

Khi cần can thiệp trực tiếp trên máy:

```bash
git pull
make ec2-restart   # build lại api + chạy migration mới, hạ tầng giữ nguyên
```

## 6. CI/CD

```
merge vào main
   └─ verify      (GitHub runner)  gofmt + vet + unit test
   └─ build-push  (GitHub runner)  build amd64 → ghcr.io/<owner>/docs-hub-api:<sha> + :latest
   └─ deploy      (runner trên EC2) compose pull → up -d migrate api → chờ /readyz
```

Bước deploy chạy trên **self-hosted runner đặt ngay trên EC2**, nên security group
không phải mở port 22 cho dải IP của GitHub, và máy không phải build gì.

Cài runner (một lần, token sống 1 giờ, lấy ở Settings → Actions → Runners):

```bash
bash deployments/ec2/setup-runner.sh https://github.com/<owner>/docs-hub-api <TOKEN> api
```

Deploy tay một tag cũ để rollback: Actions → deploy → **Run workflow** → điền
`image_tag` bằng SHA muốn quay lại.

Runner nằm trong máy nên không cần SSH key, và GHCR đăng nhập bằng
`GITHUB_TOKEN` của workflow. Secret ứng dụng được quản lý bằng GitHub Actions
Secrets như mô tả ở mục 3.

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

## Khi không SSH được vào EC2

Security group khoá port 22 theo IP cố định, mà IP nhà thường là IP động — nên
việc mất SSH là chuyện bình thường, không phải máy chết. Cách phân biệt nhanh:

```bash
curl -s -o /dev/null -w '%{http_code}\n' https://api.docshub.io.vn/swagger/index.html
```

Trả `200` mà SSH vẫn timeout thì máy đang khoẻ, vấn đề nằm ở rule port 22 (sửa
Source thành `My IP` trong security group).

Quan trọng: **deploy không phụ thuộc SSH**. Runner nằm ngay trên EC2 nên pipeline
vẫn chạy bình thường kể cả khi bạn không vào được máy — muốn deploy lại chỉ cần
vào Actions → deploy → Run workflow.
