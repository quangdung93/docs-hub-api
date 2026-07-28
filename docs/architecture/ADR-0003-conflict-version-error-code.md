# ADR-0003: Bổ sung mã lỗi CONFLICT_VERSION

- Trạng thái: Đề xuất (chờ TL duyệt)
- Ngày: 2026-07-28

## Bối cảnh
Optimistic lock cần một mã lỗi nghiệp vụ khi client gửi version cũ. Bảng `templates/04` **chưa có** mã phù hợp.

## Quyết định
Thêm mã nghiệp vụ `CONFLICT_VERSION` (nhóm `BUSINESS_RULE_*`, HTTP 200, retryable=true) vào `ERROR_CODES.md` của repo.

## Phương án dự phòng
Nếu TL từ chối thêm mã mới, dùng `SESSION_CONFLICT` (đã có trong templates/04). Chỉ cần đổi hằng số ở `internal/common/errcode/business.go` và `domain.ErrVersionConflict`.

## Việc cần làm
- [ ] TL duyệt bổ sung mã vào catalogue ISC.
- [ ] Đồng bộ vào tài liệu `ERROR_CODES.md` cấp tổ chức.
