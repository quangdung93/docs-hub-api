# Module `project`

Quản lý dự án (Project) và thành viên dự án (Project Member) — đơn vị tổ chức tài liệu trong Local RAG Documents Hub. Mỗi dự án có một Owner, có thể mời thêm thành viên với vai trò Editor/Viewer để cùng upload/truy vấn tài liệu.

## Trách nhiệm
- CRUD dự án: tạo, liệt kê dự án của user hiện tại, cập nhật thông tin chung/cấu hình RAG, xóa cứng (hard delete, cascade sang thành viên).
- Quản lý thành viên: mời (invite, trạng thái `pending`), tự xác nhận lời mời (`accept`), đổi vai trò, gỡ khỏi dự án.
- RBAC theo vai trò trong TỪNG dự án (owner/editor/viewer), có bypass cho admin hệ thống.

## Domain model (`domain/project.go`)
- `Project`: `ID, OwnerID, Name, Description, Status, Settings, CreatedAt`.
- `ProjectSettings`: `Model, TopK, ChunkSize, AllowedFormats` — cấu hình RAG, lưu JSONB. Implement `driver.Valuer`/`sql.Scanner` (stdlib, không phải gorm) để tầng repository tự map, domain không biết ORM.
- `ProjectMember`: `ID, ProjectID, UserID, Role, Status, InvitedAt, JoinedAt`.
- `Role`: `owner | editor | viewer`. `MemberStatus`: `pending | active`.

## Các API (route prefix `/internal/api/v1`, đều yêu cầu đã xác thực — `middleware.Auth`)

| Method | Path | Mô tả | Quyền (project-level RBAC) |
|---|---|---|---|
| GET    | `/projects`                          | Danh sách dự án của user hiện tại (owner hoặc thành viên active) | Đã xác thực |
| POST   | `/projects`                          | Tạo dự án mới; user hiện tại tự động là owner active            | Đã xác thực |
| PATCH  | `/projects/:id`                      | Cập nhật thông tin chung/cấu hình (partial update)              | `owner`, `editor` |
| DELETE | `/projects/:id`                      | Xóa cứng dự án (DB cascade xóa `project_members`)                | `owner` |
| GET    | `/projects/:id/members`              | Danh sách thành viên                                             | `owner`, `editor`, `viewer` |
| POST   | `/projects/:id/members`              | Mời thành viên mới (`status=pending`)                            | `owner` |
| PATCH  | `/projects/:id/members/:userId`      | Đổi vai trò thành viên                                           | `owner` |
| DELETE | `/projects/:id/members/:userId`      | Gỡ thành viên khỏi dự án                                         | `owner` |
| POST   | `/projects/:id/members/me/accept`    | Tự xác nhận lời mời (`pending` → `active`, set `joined_at`)      | Đã xác thực (chỉ chính chủ lời mời) |

Toàn bộ vai trò trên là **role trong `project_members`** — riêng của từng dự án, khác với role hệ thống (admin/user) trong JWT.

## RBAC (`internal/middleware/project_auth.go`)
`RequireProjectRole(memberRepo, allowedRoles...)`:
1. Lấy `project_id` từ path param `:id`, `user_id` từ actor trong context (đặt bởi `middleware.Auth`).
2. **Admin hệ thống** (`actor.HasRole("admin")` — role toàn cục từ JWT) **bypass toàn bộ**, kể cả khi không phải thành viên dự án.
3. Ngược lại, tra `project_members` theo `(project_id, user_id)`: phải `status=active` và `role` thuộc `allowedRoles`. **Owner luôn qua được** mọi `allowedRoles` được truyền.
4. Không tìm thấy membership hoặc sai role → `AUTH_403` (không phân biệt lý do, tránh lộ việc dự án có tồn tại hay không cho người ngoài).

## Lỗi riêng của module
- **Nghiệp vụ** (HTTP 200, `success:false`): `ALREADY_MEMBER` (mời user đã là thành viên/đang chờ xác nhận), `INVITE_NOT_PENDING` (accept một lời mời không còn ở trạng thái pending).
- **Kỹ thuật**: `PRJ_404` (không tìm thấy dự án), `MBR_404` (không tìm thấy thành viên).

## Migration
- `migrations/000004_create_projects_table.{up,down}.sql`
- `migrations/000005_create_project_members_table.{up,down}.sql` — unique `(project_id, user_id)`, `ON DELETE CASCADE` theo `project_id`.

## Port/Dependencies
- `domain.ProjectRepository`, `domain.ProjectMemberRepository` — implement bằng GORM ở `repository/postgres_project.go` (nơi DUY NHẤT trong slice này import gorm).
- `port.TxManager` — tạo dự án + thêm owner làm thành viên active nằm trong 1 transaction.
- `port.Clock` — set `joined_at` khi tạo owner / accept invite (để test được, không gọi `time.Now()` trực tiếp trong usecase).

## Test
- `domain/project_test.go` — entity, `ProjectSettings.Value()/Scan()` round-trip, business rule `Accept()`.
- `usecase/project_uc_test.go` — toàn bộ service method, mock qua `domain/mocks` + `common/port/mocks` (sinh bằng mockery, `make mocks`).
- `delivery/http/project_handler_test.go` — cả 9 API qua router thật (bao gồm middleware RBAC), assert đúng envelope ISC.
- `internal/middleware/project_auth_test.go` — riêng cho `RequireProjectRole` (owner/viewer/not-member/admin-bypass/unauthenticated).
- Chạy: `go test ./internal/module/project/... ./internal/middleware/...`

## Test thủ công qua Swagger
1. `make up` (hạ tầng docker) → `go run ./cmd/migrate -config configs/config.local.yaml up`.
2. `go run ./cmd/api -config configs/config.local.yaml`, mở `http://localhost:8081/swagger/index.html`.
3. **Lưu ý**: `owner_id`/`user_id` có khóa ngoại tới bảng `users`, nên **không dùng trực tiếp dev-token** để tạo dự án (dev-token sinh UUID ngẫu nhiên, không ứng với user thật). Luồng đúng:
   - Lấy dev-token bất kỳ (`POST /public/api/v1/auth/dev-token`) → dùng để gọi `POST /internal/api/v1/users` tạo 2 user thật.
   - `POST /public/api/v1/auth/login` (`username` = email, `password`) cho từng user → lấy JWT thật gắn đúng `user_id`.
   - Bấm **Authorize** trên Swagger UI, dán `Bearer <token>` của user muốn đóng vai (owner/invitee) rồi test lần lượt theo đúng thứ tự nghiệp vụ: Create → List → Update → Invite → (đổi token sang invitee) Accept → (đổi lại owner) đổi role/gỡ thành viên → Delete.

## Tính năng tương lai (chưa làm)
- Chuyển quyền owner (transfer ownership) cho thành viên khác.
- Archive dự án (thêm status khác `active`/`deleted` thay vì chỉ tạo mới với `active` cố định).
- Phân trang cho danh sách thành viên khi dự án có nhiều người.