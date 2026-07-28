# CODING_CONVENTIONS — docs-hub-api

Kế thừa chuẩn ISC (`templates/01_Coding_Convention.md`). Đây là phần cụ thể hóa cho Go.

## Ngôn ngữ
- Identifier: tiếng Anh. Comment/doc/message/commit: tiếng Việt (xem `CLAUDE.md`).

## Đặt tên (Go idiom, khác bảng .NET của ISC)
| Thành phần | Quy ước | Ví dụ |
|---|---|---|
| Package | ngắn, thường, không gạch | `usecase`, `errcode` |
| Interface | KHÔNG tiền tố `I` | `UserRepository`, `Service` |
| Biến/hàm export | PascalCase | `NewService`, `HTTPStatusOf` |
| Biến/hàm nội bộ | camelCase | `buildMeta`, `scopeSort` |
| File | snake_case | `user_repository.go` |

## Quy tắc bắt buộc (được lint enforce)
- Không biến global, không `init()` side-effect (`gochecknoglobals`, `gochecknoinits`).
- `domain`/`usecase` không import gin/gorm/redis (`depguard`).
- Không `c.JSON`/`c.String` ngoài `internal/common/response` (`forbidigo`).
- Luôn bọc lỗi có ngữ cảnh: `fmt.Errorf("...: %w", err)` (`wrapcheck`, `errorlint`).
- Luôn truyền `context.Context` (tham số đầu tiên) xuống DB/Redis/HTTP (`noctx`, `contextcheck`).
- Hàm ≤ 80 dòng, độ phức tạp ≤ 15 (`funlen`, `gocyclo`).

## Lỗi
- Nghiệp vụ: trả `apperr.NewBusiness(...)` / sentinel trong `domain/errors.go`.
- Kỹ thuật: `apperr.BadRequest/Unauthorized/Internal/...`.
- KHÔNG panic cho lỗi nghiệp vụ. Panic chỉ dành cho lỗi lập trình (được `Recovery` bắt).

## Test
- Unit: bảng test, tên tiếng Việt mô tả kịch bản. Bắt buộc cả happy path và lỗi.
- Mock qua `mockery` (`make mocks`). Integration qua testcontainers (build tag `integration`).
