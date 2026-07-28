# ADR-0005: Dependency Injection & giới hạn publish sự kiện

- Trạng thái: Được chấp nhận
- Ngày: 2026-07-28

## Quyết định 1: DI bằng constructor, không dùng framework
Ráp phụ thuộc bằng constructor trong `internal/bootstrap`. Không dùng `google/wire`, `dig`, hay `fx`.

### Lý do
- ~25 object cần ráp — đọc `internal/bootstrap/app.go` từ trên xuống là tài liệu sống.
- `wire` thêm bước codegen khó debug; `dig`/`fx` dùng reflection runtime -> lỗi lúc chạy thay vì lúc compile (ngược tinh thần interface-first).
- Chống god-object: mỗi feature tự ráp trong `module.go`; bootstrap chỉ thêm 1 dòng/feature.
- Zero global: config/logger/db/metrics registry đều qua constructor (`gochecknoglobals` + `gochecknoinits` enforce).

## Quyết định 2: dev-token endpoint chỉ ở local
`POST /public/api/v1/auth/dev-token` chỉ bật khi `app.enable_dev_token=true`, và loader **fail khởi động** nếu bật ngoài `env=local`. Dùng để test API cần JWT khi module auth chưa có.

## Giới hạn đã biết: publish sự kiện in-transaction
`user.created` được publish trong cùng transaction tạo user. Nếu publish lỗi -> rollback (at-most-once). Nếu commit DB xong nhưng process chết trước khi broker nhận -> **mất sự kiện**.

### Hướng khắc phục tương lai
Áp dụng **Outbox pattern**: ghi sự kiện vào bảng `outbox` trong cùng transaction, một worker riêng đọc và publish (at-least-once). Chưa làm ở boilerplate này để giữ đơn giản.
