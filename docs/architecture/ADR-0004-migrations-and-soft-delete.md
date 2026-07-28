# ADR-0004: golang-migrate + ngữ nghĩa Soft Delete

- Trạng thái: Được chấp nhận
- Ngày: 2026-07-28

## Quyết định 1: golang-migrate thay vì GORM AutoMigrate
Dùng `golang-migrate` với SQL versioned trong `migrations/`. **Không** dùng AutoMigrate.

### Lý do
- AutoMigrate **không có `down`** -> không rollback được production.
- Không đổi/xóa/rename cột, không data migration -> sớm muộn phải viết SQL tay = 2 nguồn sự thật.
- Không bảng version -> không biết prod đang ở schema nào. `schema_migrations.dirty` của golang-migrate cho biết chính xác.
- AutoMigrate chạy lúc boot -> race khi scale nhiều pod. Migration phải là **bước riêng** (`make migrate-up`, service `migrate` trong compose, initContainer trong k8s).
- Không diễn đạt được composite unique `(email, deleted_at)`, collation `utf8mb4_0900_ai_ci`.

## Quyết định 2: Unique index trên (email, deleted_at)
Unique key là `(email, deleted_at)`, **không** chỉ `(email)`.

### Lý do & giới hạn
- Nếu chỉ unique `(email)`, một email đã xóa mềm sẽ **không bao giờ** tạo lại được.
- MySQL coi mỗi `NULL` là khác nhau -> nhiều bản ghi đã xóa mềm (deleted_at khác NULL... thực ra NULL) vẫn có thể trùng email. Với bản ghi **đang sống** (deleted_at = NULL), chỉ tồn tại 1 email — đúng như mong muốn.
- Đánh đổi đã biết: hai bản ghi cùng email cùng bị xóa mềm ở đúng cùng thời điểm mili-giây có thể xung đột (hiếm). Chấp nhận được.
- `gorm.DeletedAt` chỉ xuất hiện ở `repository/model.go`; domain không biết soft delete.

## Kiểm chứng
Integration test `TestUserRepository_SoftDeleteThenRecreateEmail` khẳng định tạo lại được email đã xóa mềm.
