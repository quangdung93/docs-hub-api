# CLAUDE.md — docs-hub-api

Hướng dẫn cho AI/lập trình viên khi làm việc trong repo này.

## Repo là gì
Dịch vụ Go **đầu tiên** của ISC. Clean Architecture + DDD, feature vertical-slice. Module `user` là bản mẫu tham chiếu; các module khác (`auth`, `file`, `notification`, `tenant`) mới ở dạng scaffold.

## Quy ước ngôn ngữ
- **Code (identifier): tiếng Anh** (idiom Go).
- **Comment, doc, message lỗi, commit: tiếng Việt.**
- Commit: `type(scope): mô tả`, thêm `[AI]` khi có AI hỗ trợ. Ví dụ: `feat(user): thêm optimistic lock [AI]`.

## Ràng buộc bắt buộc (đừng phá)
1. **Envelope ISC**: `{success, data, error, meta}` — `error` số ít, không có `message` cấp gốc. Chỉ ghi response qua `internal/common/response/*`. `c.JSON` bị lint cấm ngoài đó.
2. **Tách lỗi** (ADR-0002): nghiệp vụ → `BusinessError` (HTTP 200); kỹ thuật → `TechnicalError` (4xx/5xx). Handler chỉ `c.Error(err); return`.
3. **Domain thuần**: `internal/module/*/domain` và `usecase` KHÔNG được import gin/gorm/redis (depguard chặn).
4. **Không global, không init() side-effect** (trừ code sinh tự động).
5. **Migration là SQL versioned** (golang-migrate), không AutoMigrate.

## Thêm một module mới
1. Copy cấu trúc `internal/module/user/` (domain → usecase → repository → delivery/http → module.go).
2. Thêm migration SQL vào `migrations/`.
3. Thêm 1 dòng vào `internal/bootstrap/modules.go`.
4. Viết unit test (usecase + handler) và integration test (repository).

## Lệnh hay dùng
- `make run` — chạy local. `make up`/`make down` — hạ tầng.
- `make lint test` — kiểm tra như CI. `make test-integration` — cần Docker.
- `make mocks` / `make swagger` / `make migrate-up`.

## Tài liệu nền
- `docs/architecture/ADR-*.md` — các quyết định kiến trúc.
- `README.md` — kiến trúc + hướng dẫn chạy.
- Chuẩn ISC gốc: `am-project/am-core-api/templates/01..05,07`.
