# ADR-0007: Bổ sung mã lỗi DUPLICATE_CONTENT

- Trạng thái: Đề xuất (chờ TL duyệt)
- Ngày: 2026-09-08

## Bối cảnh

Chỉ số duy nhất `uk_revisions_version_hash` / `uk_revisions_change_hash` chặn việc nạp lại đúng một nội dung vào cùng scope. Nhưng lỗi Postgres `23505` không được dịch, nên lọt qua `apperr.Internal` thành **`SYS_500`** — client tưởng hệ thống hỏng, trong khi đây là chuyện người dùng gây ra và tự sửa được (mục #5 trong báo cáo lỗi).

Theo ADR-0002 thì đây phải là lỗi **nghiệp vụ** (HTTP 200, `success=false`). Bảng `templates/04` **chưa có** mã nào mang nghĩa "nội dung đã tồn tại":

- `DUPLICATE_EMAIL` chỉ dành cho email.
- `UPLOAD_INVALID` nói về *phiên upload* hỏng/hết hạn, không phải nội dung trùng.
- `CONFLICT_VERSION` là xung đột optimistic lock, và bản thân nó cũng đang chờ duyệt (ADR-0003).

## Quyết định

Thêm mã nghiệp vụ `DUPLICATE_CONTENT` (nhóm `BUSINESS_RULE_*`, HTTP 200, `retryable=false`) vào `ERROR_CODES.md` của repo.

`retryable=false` vì gửi lại đúng file đó thì vẫn trùng. Muốn qua được phải đổi file, hoặc lưu trữ (archive) revision đang chiếm chỗ — chỉ số là **riêng phần**, `status <> 'archived'` không tính.

## Phương án dự phòng

Nếu TL từ chối thêm mã mới, dùng `UPLOAD_INVALID` (đã có trong templates/04, cùng luồng upload) kèm `details` nói rõ lý do là trùng nội dung.

Cố ý **không** chọn `CONFLICT_VERSION` làm dự phòng: lấy một mã cũng đang chờ duyệt làm chỗ lui thì không giải quyết được gì.

Chi phí đổi rất thấp — `errcode.DuplicateContent` chỉ được tham chiếu ở **đúng một chỗ** ngoài test: hàm `duplicateContentError()` trong `internal/module/document/usecase/service.go`.

## Việc cần làm

- [ ] TL duyệt bổ sung mã vào catalogue ISC.
- [ ] Đồng bộ vào tài liệu `ERROR_CODES.md` cấp tổ chức.

## Ghi chú cho người đọc sau

Lúc viết ADR này, `internal/common/errcode/business.go` có **24 mã nghiệp vụ** nhưng `ERROR_CODES.md` chỉ ghi **8**. Trong số vắng mặt có 5 mã của module `project` mà chính chú thích code tự nhận là BỔ SUNG (`ALREADY_MEMBER`, `INVITE_NOT_PENDING`, `CANNOT_MODIFY_OWNER`, `CONFIRM_NAME_MISMATCH`, `AVATAR_NOT_UPLOADED`) — **không mã nào có ADR, không mã nào được ghi vào `ERROR_CODES.md`**.

Tức là quy trình mà ADR-0003 đặt ra đã không được giữ. ADR này chỉ làm đúng cho phần của mình; **năm mã kia vẫn còn nợ**, nên xử lý ở một PR riêng.
