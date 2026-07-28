# Module `file` (scaffold)

Chưa hiện thực. Đây là module gần nhất với nghiệp vụ "document hub".

## Trách nhiệm
Upload/download/quản lý tài liệu qua MinIO (S3-compatible), presigned URL, kiểm soát loại/kích thước file.

## Endpoint dự kiến
| Method | Path | Mô tả |
|---|---|---|
| POST | `/internal/api/v1/files` | Upload (hoặc trả presigned URL để upload trực tiếp) |
| GET | `/internal/api/v1/files/{id}` | Metadata + presigned download URL |
| DELETE | `/internal/api/v1/files/{id}` | Xóa mềm |

## Mã lỗi (templates/04)
- Nghiệp vụ (HTTP 200): `FILE_TOO_LARGE`, `IMAGE_INVALID`.

## Port cần
- `port.ObjectStore` (đã có, hiện thực bằng MinIO) — điểm nối đã sẵn trong `internal/bootstrap/infra.go`.
- `port.Publisher` để phát `file.uploaded` cho module notification/quét virus.

## Cách bắt đầu
Sao chép `internal/module/user/`. Metadata file lưu ở PostgreSQL (bảng `files`), nội dung lưu ở MinIO.
