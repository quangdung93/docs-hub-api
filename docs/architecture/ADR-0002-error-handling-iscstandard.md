# ADR-0002: Tách lỗi Nghiệp vụ / Kỹ thuật theo chuẩn ISC

- Trạng thái: Được chấp nhận
- Ngày: 2026-07-28

## Bối cảnh
Chuẩn ISC (`templates/03`, `templates/04` — bắt buộc áp dụng) quy định:
- Lỗi **nghiệp vụ** trả **HTTP 200** kèm `success=false`, do service chủ động trả.
- Lỗi **kỹ thuật** trả **4xx/5xx**, do middleware xử lý.

Đây là điều lập trình viên Go dễ làm sai nhất, vì phản xạ `return err` thường dẫn tới 5xx.

## Quyết định
- Hai type riêng biệt: `apperr.BusinessError` (không có HTTP status) và `apperr.TechnicalError` (có HTTP status). **Cố tình không** chung interface có `HTTPStatus()` — gán status cho lỗi nghiệp vụ chính là nguồn gốc lỗi cần triệt tiêu.
- Điểm phân loại **duy nhất**: `internal/middleware/errorhandler.go`.
- Handler chỉ `c.Error(err); return`. Không tự phân loại, không tự ghi JSON.
- Thứ tự middleware (quan trọng): `Recovery → RequestID → Tracing → TraceIDInjector → Logging → Metrics → ErrorHandler → SecureHeaders → CORS → BodyLimit → RateLimit → Timeout → [group] Auth → RBAC`.

## Bảo vệ khỏi trôi chuẩn
- `internal/common/response/envelope_test.go`: golden test so byte JSON với ví dụ templates/03.
- `errorhandler_test.go`: khẳng định BusinessError **không bao giờ** ra ≠ 200, TechnicalError **không bao giờ** ra 200.
- `golangci-lint forbidigo`: cấm `c.JSON`/`c.String` ngoài package `response`.

## Ràng buộc không thương lượng
HTTP-200-cho-lỗi-nghiệp-vụ là **quy định tổ chức ISC**, không phải lựa chọn kỹ thuật của service này. Lập trình viên Go sẽ phản đối — đây là chỗ ghi rõ để chấm dứt tranh luận.
