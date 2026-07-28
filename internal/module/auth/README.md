# Module `auth` (scaffold)

Chưa hiện thực. Đây là bản mô tả hợp đồng dự kiến để đội phát triển bắt đầu.

## Trách nhiệm
Đăng nhập, cấp/làm mới/thu hồi JWT, xác thực đa yếu tố (tương lai). Thay thế endpoint tạm `POST /public/api/v1/auth/dev-token`.

## Endpoint dự kiến (templates/02)
| Method | Path | Mô tả |
|---|---|---|
| POST | `/public/api/v1/auth/login` | Đăng nhập, trả access + refresh token |
| POST | `/public/api/v1/auth/refresh` | Làm mới access token |
| POST | `/internal/api/v1/auth/logout` | Thu hồi token (blacklist trên Redis) |
| POST | `/public/api/v1/auth/resend-otp` | Gửi lại OTP |

## Mã lỗi (templates/04)
- Nghiệp vụ (HTTP 200): `INVALID_OTP`, `USER_LOCKED`, `INVALID_PASS`, `MFA_REQUIRED`, `SESSION_CONFLICT`.
- Kỹ thuật: `AUTH_401`, `AUTH_403`.

## Port cần
- `domain.UserRepository` (dùng lại từ module user, hoặc port `Credentials` riêng).
- `port.Cache` cho JWT blacklist / refresh-token store.
- `pkg/jwt.Manager`, `pkg/hashing.Hasher` (đã có sẵn).

## Cách bắt đầu
Sao chép cấu trúc `internal/module/user/` (domain → usecase → repository → delivery/http → module.go), rồi thêm 1 dòng vào `internal/bootstrap/modules.go`.
