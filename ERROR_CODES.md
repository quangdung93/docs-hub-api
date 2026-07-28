# ERROR_CODES — document-hub-api

Kế thừa catalogue chuẩn ISC (`templates/04`). Tài liệu này chỉ ghi phần **bổ sung** của repo.

## Nguyên tắc phân loại (ADR-0002)
| Loại | HTTP | success | Ai xử lý |
|---|---|---|---|
| Nghiệp vụ | 200 | false | Service chủ động trả `*apperr.BusinessError` |
| Kỹ thuật | 4xx/5xx | false | Middleware `errorhandler.go` |

## Mã nghiệp vụ dùng trong module user
| Mã | HTTP | Retryable | Ngữ cảnh |
|---|---|---|---|
| `DUPLICATE_EMAIL` | 200 | false | Email đã tồn tại khi tạo user |
| `USER_LOCKED` | 200 | false | Thao tác trên tài khoản bị khóa |
| `INVALID_PROFILE` | 200 | false | Giá trị trạng thái không hợp lệ |
| `CONFLICT_VERSION` ⚠️ | 200 | true | Optimistic lock — version cũ (xem ADR-0003) |

⚠️ `CONFLICT_VERSION` là mã **bổ sung**, chưa có trong catalogue ISC gốc. Đang chờ TL duyệt (ADR-0003); fallback `SESSION_CONFLICT`.

## Mã kỹ thuật dùng
`REQ_400`, `AUTH_401`, `AUTH_403`, `USR_404`, `REQ_TIMEOUT`, `RATE_429`, `SYS_500`, `DB_500`, `DB_503`, `MQ_502`, `EXT_504`.

Xem `internal/common/errcode/` để biết hằng số + bảng map HTTP status.
