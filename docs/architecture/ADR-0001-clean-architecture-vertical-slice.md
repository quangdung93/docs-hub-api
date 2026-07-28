# ADR-0001: Clean Architecture theo Vertical Slice

- Trạng thái: Được chấp nhận
- Ngày: 2026-07-28

## Bối cảnh
Yêu cầu ban đầu nêu cả hai cách bố cục xung đột: layer-first (`internal/domain`, `internal/usecase`, ...) và feature-independent slice (`internal/user`, `internal/auth`, ...). Không thể có cả hai ở cùng cấp `internal/`.

## Quyết định
Chọn **slice-first, layer bên trong mỗi slice**:

```
internal/module/user/{domain,usecase,repository,delivery/http}
```

Mọi tên tầng yêu cầu (domain/usecase/repository/delivery) vẫn tồn tại như **package Go thật**, chỉ đổi thứ tự lồng nhau.

## Lý do
1. **Ràng buộc biên dịch được.** Mỗi tầng là 1 package -> `golangci-lint depguard` chặn được `domain`/`usecase` import gin/gorm/redis. "Business logic không phụ thuộc framework" trở thành lỗi build, không phải khẩu hiệu.
2. **Không god-package.** Layer-first sẽ gom user+auth+file+notification+tenant vào chung `internal/repository/` -> merge conflict, ai cũng import được của ai (circular dependency).
3. **Feature độc lập.** Mỗi module tự trị; muốn dùng module khác phải qua port khai báo tường minh.

## Hệ quả
- Chi phí: ~60 dòng mapper domain↔model cho mỗi entity. Đây chính là cái giá để ràng buộc kiến trúc thành sự thật kiểm chứng được.
- Chiều phụ thuộc: `delivery → usecase → domain ← repository`; `domain → chỉ stdlib + common/apperr`.
