# ERROR_CODES — docs-hub-api

Kế thừa catalogue chuẩn ISC (`templates/04`). Tài liệu này chỉ ghi phần **bổ sung** của repo.

## Nguyên tắc phân loại (ADR-0002)
| Loại | HTTP | success | Ai xử lý |
|---|---|---|---|
| Nghiệp vụ | 200 | false | Service chủ động trả `*apperr.BusinessError` |
| Kỹ thuật | 4xx/5xx | false | Middleware `errorhandler.go` |

## Mã nghiệp vụ dùng trong module user/document
| Mã | HTTP | Retryable | Ngữ cảnh |
|---|---|---|---|
| `DUPLICATE_EMAIL` | 200 | false | Email đã tồn tại khi tạo user |
| `USER_LOCKED` | 200 | false | Thao tác trên tài khoản bị khóa |
| `INVALID_PROFILE` | 200 | false | Giá trị trạng thái không hợp lệ |
| `CONFLICT_VERSION` ⚠️ | 200 | true | Optimistic lock — version cũ (xem ADR-0003) |
| `FILE_TOO_LARGE` | 200 | false | File upload vượt giới hạn 50 MiB |
| `UPLOAD_INVALID` | 200 | false | Phiên upload không hợp lệ, đã hoàn tất hoặc hết hạn |
| `DOCUMENT_RETRY_INVALID` | 200 | false | Revision không ở trạng thái `failed` |
| `DUPLICATE_CONTENT` ⚠️ | 200 | false | Nội dung file đã có revision khác trong cùng scope (xem ADR-0007) |

⚠️ `CONFLICT_VERSION` là mã **bổ sung**, chưa có trong catalogue ISC gốc. Đang chờ TL duyệt (ADR-0003); fallback `SESSION_CONFLICT`.

⚠️ `DUPLICATE_CONTENT` là mã **bổ sung**, chưa có trong catalogue ISC gốc. Đang chờ TL duyệt (ADR-0007); fallback `UPLOAD_INVALID`.

> **Tài liệu này đang thiếu so với code.** `internal/common/errcode/business.go` có 24 mã nghiệp vụ, bảng trên mới ghi 9. Đáng chú ý: 5 mã của module `project` (`ALREADY_MEMBER`, `INVITE_NOT_PENDING`, `CANNOT_MODIFY_OWNER`, `CONFIRM_NAME_MISMATCH`, `AVATAR_NOT_UPLOADED`) tự nhận là **bổ sung** trong chú thích code nhưng **không có ADR và không được ghi ở đây**. Cần một PR riêng để dọn.

## Mã kỹ thuật dùng
`REQ_400`, `AUTH_401`, `AUTH_403`, `USR_404`, `REQ_TIMEOUT`, `RATE_429`, `SYS_500`, `DB_500`, `DB_503`, `MQ_502`, `EXT_504`.

Xem `internal/common/errcode/` để biết hằng số + bảng map HTTP status.
