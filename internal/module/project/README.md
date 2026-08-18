# Module `project`

Quản lý dự án (Project) và thành viên dự án (Project Member) — đơn vị tổ chức tài liệu trong Local RAG Documents Hub. Mỗi dự án có một Owner, có thể mời thêm thành viên với vai trò Editor/Viewer để cùng upload/truy vấn tài liệu.

## Trách nhiệm
- CRUD dự án: tạo, liệt kê dự án của user hiện tại, cập nhật thông tin chung/cấu hình RAG, xóa cứng (hard delete, cascade sang thành viên).
- Quản lý thành viên: mời (invite, trạng thái `pending`), tự xác nhận lời mời (`accept`), đổi vai trò, gỡ khỏi dự án.
- RBAC theo vai trò trong TỪNG dự án (owner/editor/viewer), có bypass cho admin hệ thống.

## Domain model (`domain/project.go`)
- `Project`: `ID, OwnerID, Name, Description, Status, Settings, AvatarKey, CreatedAt`. `AvatarKey` là object key ảnh đại diện trong MinIO, rỗng nếu chưa có ảnh.
- `ProjectSettings`: `Model, TopK, ChunkSize, AllowedFormats` — cấu hình RAG, lưu JSONB. Implement `driver.Valuer`/`sql.Scanner` (stdlib, không phải gorm) để tầng repository tự map, domain không biết ORM.
- `ProjectMember`: `ID, ProjectID, UserID, Role, Status, InvitedAt, JoinedAt`.
- `Role`: `owner | editor | viewer`. `MemberStatus`: `pending | active`.

## RBAC theo vai trò dự án — ĐANG TẮT TẠM THỜI
Quyết định chung của team (chờ thiết kế lại theo mô hình Group/Permission kiểu Django):
`middleware.RequireProjectRole` bị comment lại tại các route ở `Register()` (`delivery/http/project_handler.go`) — code RBAC (`internal/middleware/project_auth.go`) vẫn còn nguyên, chỉ chưa gắn vào route. Mọi route dưới đây hiện **chỉ yêu cầu đã xác thực** (`middleware.Auth`), KHÔNG kiểm tra vai trò owner/editor/viewer. Cột "Quyền (project-level RBAC)" mô tả thiết kế DỰ KIẾN khi bật lại.

## Các API (route prefix `/internal/api/v1`, đều yêu cầu đã xác thực — `middleware.Auth`)

| Method | Path | Mô tả | Quyền (project-level RBAC, đang tắt) |
|---|---|---|---|
| GET    | `/projects`                          | Danh sách dự án của user hiện tại (owner hoặc thành viên active) | Đã xác thực |
| POST   | `/projects`                          | Tạo dự án mới; user hiện tại tự động là owner active            | Đã xác thực |
| PATCH  | `/projects/:id`                      | Cập nhật thông tin chung/cấu hình (partial update)              | `owner` |
| DELETE | `/projects/:id`                      | Xóa cứng dự án — **yêu cầu body `{"confirm_name": "<tên dự án>"}` khớp CHÍNH XÁC** tên hiện tại (BR SRS: không thể hoàn tác, cần xác nhận), DB cascade xóa `project_members` | `owner` |
| POST   | `/projects/:id/avatar/upload-url`    | Xin phép upload ảnh đại diện — trả presigned PUT URL (xem Luồng upload ảnh đại diện) | `owner` |
| POST   | `/projects/:id/avatar/complete`      | Xác nhận đã upload ảnh xong (`avatar_key` được gán sau khi xác minh qua MinIO) | `owner` |
| GET    | `/projects/:id/members`              | Danh sách thành viên                                             | `owner`, `editor`, `viewer` |
| POST   | `/projects/:id/members`              | Mời thành viên mới (`status=pending`)                            | `owner` |
| PATCH  | `/projects/:id/members/:userId`      | Đổi vai trò thành viên — **không áp dụng cho owner** (xem Ràng buộc nghiệp vụ) | `owner` |
| DELETE | `/projects/:id/members/:userId`      | Gỡ thành viên khỏi dự án — **không áp dụng cho owner** (xem Ràng buộc nghiệp vụ) | `owner` |
| POST   | `/projects/:id/members/me/accept`    | Tự xác nhận lời mời (`pending` → `active`, set `joined_at`)      | Đã xác thực (chỉ chính chủ lời mời) |

Toàn bộ vai trò trên là **role trong `project_members`** — riêng của từng dự án, khác với role hệ thống (admin/user) trong JWT.

## Luồng upload ảnh đại diện (presigned URL — giống hệt kiến trúc module `file`)
Backend không đi qua bytes ảnh. `ProjectResponse.avatar_url` (mọi response trả về `Project`) là presigned GET URL, rỗng nếu dự án chưa có ảnh.
1. `POST /projects/:id/avatar/upload-url` — body `{"mime_type", "size_bytes"}`; validate định dạng (PNG/JPEG/WebP) + dung lượng (`project.avatar_max_bytes`, xem `configs/*.yaml` — TẠM THỜI 5 MiB, chờ team chốt), dự án phải đã tồn tại. Trả `upload_url` (presigned PUT, hết hạn theo `project.avatar_presigned_ttl`) trỏ tới object key CỐ ĐỊNH `projects/{id}/avatar` (1 slot/dự án — upload lại chỉ ghi đè, không rác).
2. FE `PUT` ảnh trực tiếp lên `upload_url` (thẳng tới MinIO).
3. `POST /projects/:id/avatar/complete` — backend gọi `Stat` xác nhận object thực sự tồn tại rồi mới gán `avatar_key` vào dự án. Idempotent.

## Ràng buộc nghiệp vụ (theo URD/SRS)
1. **Mỗi dự án có đúng một Owner; chỉ Owner hoặc Admin hệ thống được xóa dự án.** → `RequireProjectRole(memberRepo, RoleOwner)` trên route Delete + admin bypass.
2. **Owner toàn quyền, Editor chỉ upload+query, Viewer chỉ query.** → Sửa cấu hình/thông tin dự án (`PATCH /projects/:id`) là hành vi quản trị, **chỉ Owner** — Editor KHÔNG được (dù có quyền upload tài liệu ở module `file` sau này).
3. **Thành viên `pending` chưa được truy cập tài liệu.** → `RequireProjectRole` bắt buộc `status=active`; thành viên mới mời luôn ở `pending` tới khi tự `accept`.
4. **Xóa dự án không thể hoàn tác, cần xác nhận.** → `DELETE /projects/:id` bắt buộc body `confirm_name` khớp đúng tên dự án hiện tại; sai tên → lỗi nghiệp vụ `CONFIRM_NAME_MISMATCH`, không xóa. (Đảm bảo enforce ở backend, không chỉ dựa vào FE hiện popup xác nhận — tránh bị bypass qua gọi API trực tiếp.)
5. **[Bổ sung, không có trong URD/SRS gốc nhưng cần thiết để BR #1 luôn đúng]** Owner **không thể tự đổi role hoặc tự gỡ chính mình** qua `PATCH|DELETE /projects/:id/members/:userId` — nếu cho phép, dự án sẽ mất hoàn toàn owner trong `project_members` dù `projects.owner_id` vẫn trỏ đúng người đó, khiến chính Owner bị khóa quyền quản lý dự án của mình (phát hiện qua test thực tế, xem lỗi `CANNOT_MODIFY_OWNER`). Muốn đổi chủ dự án cần API transfer ownership riêng (chưa hiện thực — xem mục Tính năng tương lai).

## RBAC (`internal/middleware/project_auth.go`) — code còn nguyên, ĐANG KHÔNG gắn vào route (xem mục trên)
`RequireProjectRole(memberRepo, allowedRoles...)`:
1. Lấy `project_id` từ path param `:id`, `user_id` từ actor trong context (đặt bởi `middleware.Auth`).
2. **Admin hệ thống** (`actor.HasRole("admin")` — role toàn cục từ JWT) **bypass toàn bộ**, kể cả khi không phải thành viên dự án.
3. Ngược lại, tra `project_members` theo `(project_id, user_id)`: phải `status=active` và `role` thuộc `allowedRoles`. **Owner luôn qua được** mọi `allowedRoles` được truyền.
4. Không tìm thấy membership hoặc sai role → `AUTH_403` (không phân biệt lý do, tránh lộ việc dự án có tồn tại hay không cho người ngoài).

## Lỗi riêng của module
- **Nghiệp vụ** (HTTP 200, `success:false`): `ALREADY_MEMBER` (mời user đã là thành viên/đang chờ xác nhận), `INVITE_NOT_PENDING` (accept một lời mời không còn ở trạng thái pending), `CANNOT_MODIFY_OWNER` (đổi role/gỡ owner qua API quản lý thành viên), `CONFIRM_NAME_MISMATCH` (xóa dự án nhưng `confirm_name` không khớp tên hiện tại), `IMAGE_INVALID` (định dạng ảnh đại diện không hỗ trợ), `FILE_TOO_LARGE` (ảnh đại diện vượt giới hạn dung lượng), `AVATAR_NOT_UPLOADED` (xác nhận upload ảnh nhưng ảnh chưa thực sự lên storage — retryable).
- **Kỹ thuật**: `PRJ_404` (không tìm thấy dự án), `MBR_404` (không tìm thấy thành viên).

## Migration
- `migrations/000004_create_projects_table.{up,down}.sql`
- `migrations/000005_create_project_members_table.{up,down}.sql` — unique `(project_id, user_id)`, `ON DELETE CASCADE` theo `project_id`.
- `migrations/000006_add_project_avatar.{up,down}.sql` — thêm cột `avatar_key` (nullable) vào `projects`.

## Port/Dependencies
- `domain.ProjectRepository`, `domain.ProjectMemberRepository` — implement bằng GORM ở `repository/postgres_project.go` (nơi DUY NHẤT trong slice này import gorm).
- `port.TxManager` — tạo dự án + thêm owner làm thành viên active nằm trong 1 transaction.
- `port.Clock` — set `joined_at` khi tạo owner / accept invite; sinh `expires_at` khi xin presigned URL ảnh đại diện (để test được, không gọi `time.Now()` trực tiếp trong usecase).
- `port.ObjectStore` — presigned PUT/GET URL + `Stat` xác nhận upload ảnh đại diện (implement bằng MinIO, điểm nối sẵn trong `internal/bootstrap/infra.go`).

## Test
- `domain/project_test.go` — entity, `ProjectSettings.Value()/Scan()` round-trip, business rule `Accept()`.
- `usecase/project_uc_test.go` — toàn bộ service method (bao gồm luồng ảnh đại diện: `RequestAvatarUpload`, `CompleteAvatarUpload`, `AvatarURL`), `ensureNotOwner` (chặn đổi role/gỡ owner) và `Delete` xác nhận tên (`ErrConfirmNameMismatch`); mock qua `domain/mocks` + `common/port/mocks` (sinh bằng mockery, `make mocks`).
- `delivery/http/project_handler_test.go` — toàn bộ API qua router thật, assert đúng envelope ISC; có test riêng cho luồng ảnh đại diện, thiếu/sai `confirm_name` khi xóa.
- `internal/middleware/project_auth_test.go` — riêng cho `RequireProjectRole` (owner/viewer/not-member/admin-bypass/unauthenticated) — middleware còn test được dù chưa gắn vào route thật.
- Chạy: `go test ./internal/module/project/... ./internal/middleware/...`

## Test thủ công qua Swagger
1. `make up` (hạ tầng docker) → `go run ./cmd/migrate -config configs/config.local.yaml up`.
2. `go run ./cmd/api -config configs/config.local.yaml`, mở `http://localhost:8081/swagger/index.html`.
3. **Lưu ý**: `owner_id`/`user_id` có khóa ngoại tới bảng `users`, nên **không dùng trực tiếp dev-token** để tạo dự án (dev-token sinh UUID ngẫu nhiên, không ứng với user thật). Luồng đúng:
   - Lấy dev-token bất kỳ (`POST /public/api/v1/auth/dev-token`) → dùng để gọi `POST /internal/api/v1/users` tạo 2 user thật.
   - `POST /public/api/v1/auth/login` (`username` = email, `password`) cho từng user → lấy JWT thật gắn đúng `user_id`.
   - Bấm **Authorize** trên Swagger UI, dán `Bearer <token>` của user muốn đóng vai (owner/invitee) rồi test lần lượt theo đúng thứ tự nghiệp vụ: Create → List → Update → Invite → (đổi token sang invitee) Accept → (đổi lại owner) đổi role/gỡ thành viên → Delete (nhớ kèm body `{"confirm_name": "<đúng tên dự án>"}`).
4. **Test luồng ảnh đại diện** (sau khi đã có project ID từ bước Create):
   - `POST /projects/{id}/avatar/upload-url` với body `{"mime_type": "image/png", "size_bytes": 12345}` → lấy `upload_url`.
   - `PUT` file ảnh thật lên `upload_url` (ngoài Swagger UI — dùng curl/Postman, vì đây là request thẳng tới MinIO không qua backend).
   - `POST /projects/{id}/avatar/complete` → response trả `avatar_url` (presigned GET, xem được ảnh trực tiếp qua URL này).
   - Từ giờ mọi response trả `Project` (Create/List/Update/complete) đều kèm `avatar_url`.

## Tính năng tương lai (chưa làm)
- Chuyển quyền owner (transfer ownership) cho thành viên khác — cần thiết vì hiện tại KHÔNG có cách nào đổi chủ dự án (ràng buộc nghiệp vụ #5 chặn cả đổi role lẫn gỡ owner).
- Archive dự án (thêm status khác `active`/`deleted` thay vì chỉ tạo mới với `active` cố định).
- Phân trang cho danh sách thành viên khi dự án có nhiều người.