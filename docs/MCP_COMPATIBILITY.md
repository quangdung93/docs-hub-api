# MCP compatibility

## Contract v1

- SDK: `github.com/modelcontextprotocol/go-sdk` v1.6.1 (stable).
- Protocols: 2025-11-25, 2025-06-18, 2025-03-26 và 2024-11-05 theo bảng tương thích của SDK.
- Transport: stateless Streamable HTTP tại `POST /mcp`, JSON response.
- Authentication: access token hiện tại trong `Authorization: Bearer <token>`; local dùng local actor giống internal REST API.
- Capability: resources và tools read-only. Upload, delete và mọi mutation không được expose trong v1.

MCP là delivery adapter và chỉ gọi application usecase. Tool/resource contract được xem là public API; thay đổi breaking phải tạo tên mới hoặc giữ adapter tương thích.

## Bật MCP

Đặt `APP_MCP_ENABLED=true` cho API và giữ `APP_RAGFLOW_ENABLED=true`, sau đó khởi động lại service. Các giới hạn có thể cấu hình bằng `APP_MCP_REQUESTS_PER_WINDOW`, `APP_MCP_WINDOW`, `APP_MCP_MAX_SOURCE_LINES` và `APP_MCP_MAX_EXCERPT_CHARS`.

## Client rollout

Contract được thiết kế cho MCP client hỗ trợ Streamable HTTP như Codex, Claude và VS Code. Trước production cần chạy handshake, `tools/list`, `resources/templates/list` và một call có citation bằng client mục tiêu thực tế.

## Security limits

- ACL được kiểm tra lại trong project/document/retrieval/chat usecase.
- Rate limit theo principal và tool qua Redis; lỗi Redis fail-open nhưng được log.
- Source bị giới hạn số dòng và số ký tự theo `mcp.*` config.
- Lỗi trả client chỉ chứa code/message an toàn, không chứa SQL, object key, prompt hay stack trace.
