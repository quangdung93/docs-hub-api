# Module `auth`

Đảm nhiệm chức năng xác thực người dùng (Authentication) và cấp phát token.

## Trách nhiệm
Xử lý luồng đăng nhập nội bộ (không có đăng ký tài khoản công khai), quản lý phiên đăng nhập qua JWT và Cookie, hỗ trợ đăng xuất và truy xuất thông tin tài khoản hiện tại.

## Các API hiện tại
| Method | Path | Mô tả |
|---|---|---|
| POST | `/public/api/v1/auth/login` | Đăng nhập bằng `username`/`password`. Trả về thông tin User, token và tự động set Cookie `access_token` (HttpOnly). |
| POST | `/internal/api/v1/auth/logout` | Đăng xuất. Xóa Cookie ở client và thu hồi token. |
| GET | `/internal/api/v1/auth/me` | Lấy thông tin user hiện tại. Hỗ trợ xác thực qua Cookie hoặc Header `Authorization: Bearer`. |

## Các tính năng tương lai (cần phát triển thêm)
- API Làm mới token (`/refresh`)
- Tính năng gửi OTP/Xác thực đa yếu tố (MFA).

## Port/Dependencies cần thiết
- `domain.UserRepository`: Truy vấn thông tin người dùng từ DB.
- `domain.SessionRepository`: Lưu trữ phiên đăng nhập vào DB.
- `jwt.Manager`: Quản lý sinh và xác thực JSON Web Token.- `pkg/jwt.Manager`, `pkg/hashing.Hasher` (đã có sẵn).

## Cách bắt đầu
Sao chép cấu trúc `internal/module/user/` (domain → usecase → repository → delivery/http → module.go), rồi thêm 1 dòng vào `internal/bootstrap/modules.go`.
