# Project Knowledge Hub RAG Dev Guide

## Source Inputs

- `Agents.md`: không tồn tại trong repository tại thời điểm lập guide.
- Nguồn quy ước thay thế: `CLAUDE.md`, `CODING_CONVENTIONS.md`, `README.md`, `docs/PROJECT_STRUCTURE.md`, `ERROR_CODES.md` và `docs/architecture/ADR-0001..0006`.
- Requirement analysis: mô tả trực tiếp của người dùng ngày 2026-08-10 về kho tri thức theo dự án, version/change request, hỏi đáp có trích nguồn, LocalAI và MCP.
- Optional references đã kiểm tra:
  - `internal/module/user/`: vertical slice mẫu hoàn chỉnh.
  - `internal/module/auth/README.md`, `internal/module/file/README.md`, `internal/module/tenant/README.md`: scaffold và hợp đồng dự kiến.
  - `migrations/000001_create_users_table.*.sql`, `migrations/000002_enable_pgvector.*.sql`.
  - LocalAI official docs: <https://localai.io/docs/overview/index.html>, <https://localai.io/getting-started/models/index.html>.
  - Official MCP Go SDK: <https://github.com/modelcontextprotocol/go-sdk>.

## Guide Summary

- Goal: xây dựng backend Go cho một kho tri thức RAG giống NotebookLM, cô lập dữ liệu theo dự án, hiểu ngữ cảnh version/change request, trả lời bằng ngôn ngữ tự nhiên và luôn kèm nguồn có thể kiểm chứng.
- User/business outcome:
  - Người dùng đăng nhập, tạo hoặc mở dự án được cấp quyền.
  - Mỗi dự án có nhiều version phát hành và change request theo dòng thời gian.
  - Người dùng tải tài liệu vào đúng version/change request; hệ thống xử lý bất đồng bộ thành các chunk có vector.
  - Câu hỏi về trạng thái hiện tại mặc định dùng version đã phát hành mới nhất.
  - Câu hỏi về “thay đổi qua các version/change request” phải tổng hợp theo thứ tự thời gian, không trộn thành một câu trả lời mất dấu phiên bản.
  - Mỗi khẳng định quan trọng có citation gồm tài liệu, version/change request, dòng hoặc trang và URL xem bản gốc.
  - Các ứng dụng AI khác có thể truy cập cùng kho tri thức qua MCP nhưng vẫn tuân thủ quyền dự án.
- Primary code area: các vertical slice mới trong `internal/module`, hạ tầng LocalAI/RabbitMQ/PostgreSQL/MinIO, migration, config, worker và MCP server.
- In scope: I. đăng nhập; II. tạo dự án; III. danh sách dự án; IV. tra cứu/hỏi đáp; V. tải tài liệu; VI. quản lý tài liệu; VII. cổng MCP; ingestion; chunking; embedding; hybrid retrieval; citation; quyền dự án; audit tối thiểu; quan sát và kiểm thử.
- Out of scope v1: frontend hoàn chỉnh, cộng tác chỉnh sửa tài liệu trực tuyến, OCR chất lượng cao cho mọi ngôn ngữ, fine-tune model, vector database riêng, Elasticsearch bắt buộc, multi-region, billing/quota thương mại và tự động đọc repository Git.
- Existing behavior that must remain unchanged:
  - Envelope ISC `{success,data,error,meta}` và quy tắc BusinessError HTTP 200 / TechnicalError 4xx-5xx.
  - `user` module và các API hiện hữu.
  - Clean Architecture, domain/usecase không phụ thuộc Gin/GORM/Redis/MinIO/LocalAI SDK.
  - Constructor DI, không global, không `init()` side effect, migration bằng `golang-migrate`.
  - Dev-token chỉ tồn tại ở local và sẽ không bị dùng làm đăng nhập production.
- Quyết định kiến trúc v1:
  - Toàn bộ code do dự án sở hữu viết bằng Go; bổ sung `cmd/worker` cho ingestion và `cmd/mcp` nếu cần stdio transport.
  - PostgreSQL + pgvector thực hiện semantic search và PostgreSQL full-text search; chưa dùng Elasticsearch để giảm RAM/CPU và độ phức tạp vận hành.
  - LocalAI được gọi qua OpenAI-compatible HTTP API. Một chat model nhỏ (Qwen quantized cỡ khoảng 3B theo mong muốn) dùng cho intent/rewrite/answer; một embedding model đa ngôn ngữ nhỏ dùng riêng cho vector.
  - Tài liệu gốc ở MinIO; canonical extracted text có đánh số dòng và mapping trang ở PostgreSQL/MinIO để citation ổn định.
  - Truy hồi luôn lọc `project_id` và quyền thành viên trước khi xếp hạng; không bao giờ lấy top-K toàn hệ thống rồi mới lọc quyền.

## Development Readiness

### Ready For Development

- Dùng `internal/module/user` làm mẫu cho mọi vertical slice mới.
- Hiện thực đăng nhập bằng user/password hiện có, access token ngắn hạn và refresh session có thể thu hồi.
- Hiện thực project, project membership và quyền `owner`, `editor`, `viewer`.
- Hiện thực project version, change request, document, document revision và ingestion job.
- Upload file gốc vào MinIO, lưu metadata PostgreSQL, publish job RabbitMQ và xử lý trong worker Go.
- Hỗ trợ `.txt`, `.md` và `.pdf` có text layer trong v1; thiết kế parser dạng port để bổ sung `.docx` sau.
- Chunk theo cấu trúc, tạo embedding bằng LocalAI, lưu pgvector và `tsvector`.
- Hybrid retrieval, intent routing, query rewrite, generation có citation và nguyên tắc từ chối khi thiếu bằng chứng.
- REST API đồng bộ trả full response qua ISC envelope.
- MCP server dùng official Go SDK, expose resources/tools chỉ bằng cách gọi lại application usecase.
- Unit test, repository integration test, API integration test và một bộ RAG evaluation nhỏ bằng dữ liệu fixture không nhạy cảm.

### Blocked / Pending

- **P0 - model contract:** phải chọn chính xác chat model, embedding model, quantization và embedding dimension trước khi tạo cột `vector(N)` và HNSW index. Không hardcode tên “qwen3b”; model ID phải qua config. Nếu đổi embedding dimension cần migration/re-index toàn bộ.
- **P0 - định nghĩa latest:** guide giả định “mới nhất” là version có trạng thái `published` với `sequence_no` lớn nhất; draft version và open change request không ảnh hưởng câu trả lời mặc định. Product owner phải xác nhận nếu accepted change request chưa release cũng được tính vào latest.
- **P0 - lifecycle version/CR:** cần xác nhận một tài liệu có thể gắn đồng thời với version và change request hay không. Guide chọn XOR: mỗi document revision thuộc đúng một version hoặc đúng một change request.
- **P0 - quyền:** guide chọn ACL theo project (`owner/editor/viewer`) trong v1. Multi-tenant và PostgreSQL RLS chưa bật vì chưa có hợp đồng tenant trong JWT.
- **P1 - loại file:** `.docx`, `.xlsx`, `.pptx`, HTML và ảnh scan cần chốt thư viện/parser/OCR, giới hạn dung lượng và yêu cầu bảo toàn layout trước khi triển khai.
- **P1 - citation PDF:** “dòng” của PDF không ổn định giữa các trình đọc. Guide dùng dòng của canonical extracted text và bổ sung trang PDF; UI phải hiển thị rõ đây là dòng của bản text trích xuất.
- **P1 - MCP protocol rollout:** khóa một phiên bản stable của official Go SDK trong `go.mod`, ghi protocol versions hỗ trợ và kiểm tra client mục tiêu trước production. Không phụ thuộc bản prerelease nếu chưa có quyết định.
- **P1 - streaming answer:** streaming sẽ cần ngoại lệ có tài liệu cho quy tắc ISC envelope. V1 dùng response đồng bộ; SSE/streaming là phase sau.
- **P1 - ADR-0006 conflict:** ADR-0006 đang quyết định Python RAG Service + Elasticsearch + Azure OpenAI/private. Cần ADR-0007 được phê duyệt để thay thế các phần này bằng Go worker + PostgreSQL hybrid + LocalAI. Không âm thầm sửa ADR-0006.
- **P2 - SSO/MFA/forgot-password:** ngoài phạm vi login cơ bản đến khi có IdP và hợp đồng bảo mật.

## Rules From Agents.md

Do repo không có `Agents.md`, các quy tắc dưới đây được tổng hợp từ tài liệu thay thế và phải được xem như bắt buộc:

- Identifier, package, API field dùng tiếng Anh theo Go idiom; comment/doc/message lỗi dùng tiếng Việt.
- Chỉ comment phần phức tạp, lý do nghiệp vụ, invariant, thuật toán hoặc contract. Không comment lặp lại statement hiển nhiên như gán biến, `if err != nil` hoặc `return`.
- Với code học tập, mỗi file/package mới cần package doc; exported type/function cần comment giải thích vai trò. Những luồng khó như ACL, state transition, chunk mapping, RRF và idempotency phải có comment tiếng Việt gần code.
- Handler mỏng: bind/validate DTO, gọi service, trả lỗi bằng `c.Error(err); return`; chỉ `internal/common/response` được ghi JSON.
- Domain/usecase chỉ phụ thuộc stdlib, `uuid` và common port/type phù hợp; không import Gin, GORM, pgx, RabbitMQ, Redis, MinIO hay SDK LocalAI/MCP.
- Repository ánh xạ domain entity ↔ GORM model bằng mapper riêng; không đưa tag GORM vào domain.
- Mọi I/O nhận `context.Context` ở tham số đầu; lỗi phải wrap bằng `%w` với ngữ cảnh.
- Không global mutable state, không `init()` side effect, hàm không quá 80 dòng, cyclomatic complexity không quá 15.
- Migration SQL có cặp up/down; không AutoMigrate; xóa nghiệp vụ ưu tiên soft delete và optimistic lock.
- Unit test theo bảng, tên test tiếng Việt, có happy path và failure path; repository integration test dùng testcontainers.
- Mọi API REST dùng ISC envelope; lỗi nghiệp vụ HTTP 200, lỗi kỹ thuật dùng 4xx/5xx.

## Requirement Summary From NotebookLM

Không có file NotebookLM riêng. Nội dung sau là chuẩn hóa từ yêu cầu người dùng:

1. Đăng nhập và bảo vệ dữ liệu theo người dùng/quyền dự án.
2. Tạo dự án và quản lý kho tri thức riêng của từng dự án.
3. Hiển thị danh sách dự án người dùng được phép truy cập.
4. Hỏi đáp tài liệu theo ngôn ngữ tự nhiên, có hội thoại, truy hồi tối ưu và citation chính xác.
5. Tải tài liệu theo version phát triển hoặc change request.
6. Xem trạng thái xử lý, metadata, phiên bản, retry và xóa tài liệu an toàn.
7. Cung cấp MCP resources/tools cho client bên ngoài.
8. AI tham gia ba vị trí: hỗ trợ chunking theo ngữ nghĩa, phân loại/viết lại query và sinh câu trả lời có căn cứ.
9. Câu hỏi về thay đổi phải truy hồi có bao phủ từng mốc thời gian; câu hỏi về cách chức năng hoạt động mặc định dùng version published mới nhất.
10. Ưu tiên local/private, chạy được trên máy yếu.

## Existing System Context

- Go module hiện tại: `github.com/quangdung93/docs-hub-api`, Go 1.25.
- Stack đã có: Gin, GORM/PostgreSQL 16, pgvector extension, Redis, RabbitMQ, MinIO, Viper, JWT, bcrypt, OpenTelemetry, Prometheus và Swagger.
- Chỉ `user` hoàn chỉnh; `auth`, `file`, `tenant`, `notification` mới là scaffold.
- JWT hiện chỉ có access token, chưa có `jti`, token type hoặc refresh-token persistence.
- `port.ObjectStore` hỗ trợ Put/Get/PresignedGetURL/Delete nhưng Put nhận `[]byte`; upload file lớn cần bổ sung streaming port để không giữ toàn bộ file trong RAM.
- RabbitMQ hiện mới có publisher, chưa có consumer/reconnect worker.
- pgvector extension đã bật nhưng chưa có bảng chunk/index.
- ADR-0006 đã nhận diện đúng các rủi ro permission-aware retrieval, data residency, citation và re-index; topology/model serving cần được cập nhật theo yêu cầu mới.

## Contract Alignment

- Source of truth theo thứ tự:
  1. Yêu cầu sản phẩm đã xác nhận trong phần `Development Readiness`.
  2. ADR trạng thái accepted, trừ phần được ADR mới thay thế rõ ràng.
  3. `CLAUDE.md`, `CODING_CONVENTIONS.md`, chuẩn ISC và module `user`.
  4. Guide này và các giả định bảo thủ đã ghi.
- API/data/schema/route/command/job contracts: các hợp đồng đề xuất bên dưới là baseline v1; thay đổi field public sau khi frontend/MCP dùng phải có versioning hoặc backward compatibility.
- Backward compatibility requirements:
  - Không đổi route `/internal/api/v1/users` và `/public/api/v1/auth/dev-token` hiện có.
  - Dev-token tiếp tục chỉ local; login mới không phụ thuộc dev-token.
  - Không đổi shape ISC envelope.
  - Migration chỉ bổ sung bảng/cột/index; không rewrite migration `000001`/`000002` đã tồn tại.
- Permissions, auth, audit, or rollout constraints:
  - Mọi project/document/chat/retrieval route nằm dưới internal group và yêu cầu JWT.
  - Repository query luôn nhận actor/project scope; kiểm tra thành viên ở usecase và áp filter project ở SQL.
  - `owner`: quản lý project/member/version/CR/document; `editor`: version/CR/upload/manage/query; `viewer`: list/view/query/download.
  - Xóa/re-index/đổi trạng thái phải ghi audit event tối thiểu gồm actor, action, entity, timestamp và request ID.
- Ambiguities and conservative assumptions:
  - Một installation là single organization; ACL project thay cho tenant.
  - Version label là chuỗi (`v1.2.0`, `2026.08`, ...) nhưng thứ tự dùng `sequence_no` do hệ thống cấp, không sort lexical.
  - Version/CR đã published/accepted là immutable; sửa tài liệu tạo document revision hoặc scope mới.
  - Câu trả lời không đủ nguồn phải nói rõ “không tìm thấy đủ thông tin”, không dùng kiến thức model để bù.

### Permission Matrix

| Operation | owner | editor | viewer |
|---|---:|---:|---:|
| Xem/list/search/ask/download | Có | Có | Có |
| Upload/retry/archive document | Có | Có | Không |
| Tạo/sửa draft version hoặc CR | Có | Có | Không |
| Publish version/accept CR | Có | Có, nếu policy cho phép | Không |
| Sửa project, member, xóa project | Có | Không | Không |
| Gọi MCP read/query tools | Có | Có | Có |
| Gọi MCP mutation tools (nếu bật phase sau) | Có | Theo tool | Không |

## Data Model And Migrations

Tạo migration tuần tự sau `000002`; số migration thực tế phải lấy bằng `make migrate-create` để tránh trùng nhánh.

### Core tables

- `auth_sessions`
  - `id UUID PK`, `user_id UUID FK users`, `refresh_token_hash VARCHAR`, `expires_at`, `revoked_at`, `created_at`, `last_used_at`, `user_agent`, `ip_hash`.
  - Chỉ lưu hash refresh token; rotate token sau mỗi lần refresh; unique active token hash.
- `projects`
  - `id`, `code`, `name`, `description`, `status(active|archived)`, `owner_id`, `version` optimistic lock, `ragflow_dataset_id NULL`, `ragflow_sync_status`, `ragflow_last_error`, timestamps, `deleted_at`.
  - Partial unique index trên `lower(code)` khi chưa xóa.
  - Unique index trên `ragflow_dataset_id` khi khác NULL; remote ID chỉ là internal reference và không được expose cho frontend.
- `project_members`
  - `(project_id,user_id)` unique, `role(owner|editor|viewer)`, timestamps.
  - Owner cũng phải có một row membership để query thống nhất.
- `project_versions`
  - `id`, `project_id`, `label`, `sequence_no`, `status(draft|published|archived)`, `released_at`, `created_by`, timestamps.
  - Unique `(project_id,label)` và `(project_id,sequence_no)`; partial index tìm latest published.
- `change_requests`
  - `id`, `project_id`, `code`, `title`, `description`, `base_version_id`, `target_version_id NULL`, `status(draft|review|accepted|rejected)`, `sequence_no`, `accepted_at`, `created_by`, timestamps.
  - Unique `(project_id,code)` và sequence dùng để sắp theo timeline trong cùng project.
- `documents`
  - Logical document: `id`, `project_id`, `title`, `document_key`, `description`, `source_type(upload|url)`, `external_source_url NULL`, `created_by`, `version`, timestamps, `deleted_at`.
  - Unique `(project_id,document_key)` khi active.
- `document_revisions`
  - Immutable imported artifact: `id`, `document_id`, `project_id`, `project_version_id NULL`, `change_request_id NULL`, `revision_no`, `file_name`, `media_type`, `size_bytes`, `sha256`, `object_key`, `canonical_text_key NULL`, `status(uploaded|queued|processing|ready|failed|archived)`, `parser_version`, `embedding_model`, `embedding_version`, `error_code`, `error_detail_sanitized`, `created_by`, timestamps.
  - CHECK đúng một trong `project_version_id` hoặc `change_request_id` khác NULL.
  - Unique chống import lặp dựa trên `(project_id, scope, sha256)` theo policy đã xác nhận.
- `ingestion_jobs`
  - `id`, `document_revision_id`, `status(pending|running|succeeded|failed|dead)`, `attempt`, `max_attempts`, `available_at`, `locked_at`, `worker_id`, `last_error`, timestamps.
  - Unique active job cho một revision; hỗ trợ idempotency và retry có backoff.
- `document_chunks`
  - `id`, `project_id`, `document_id`, `document_revision_id`, `project_version_id NULL`, `change_request_id NULL`, `ordinal`, `content`, `content_tsv tsvector`, `embedding vector(N)`, `heading_path JSONB`, `line_start`, `line_end`, `page_start NULL`, `page_end NULL`, `token_count`, `content_hash`, timestamps.
  - Index B-tree theo project/scope/revision, GIN trên `content_tsv`, HNSW cosine trên embedding sau khi dimension/model được chốt.
  - Không copy ACL dạng JSON vào chunk ở v1; ACL kế thừa project và luôn join/filter project membership.
- `conversations`
  - `id`, `project_id`, `user_id`, `title`, `active_scope JSONB`, timestamps, `deleted_at`.
- `messages`
  - `id`, `conversation_id`, `role(user|assistant)`, `content`, `intent`, `resolved_scope JSONB`, `model`, `prompt_version`, `latency_ms`, `created_at`.
- `message_citations`
  - `id`, `message_id`, `chunk_id`, `citation_order`, `quoted_text`, `document_title_snapshot`, `scope_label_snapshot`, `line_start`, `line_end`, `page_start`, `page_end`, `source_url`.
  - Snapshot giúp lịch sử chat vẫn giải thích được citation nếu metadata tài liệu đổi; link phải kiểm tra quyền lúc truy cập.
- `audit_logs`
  - `id`, `actor_user_id`, `project_id`, `action`, `entity_type`, `entity_id`, `request_id`, `metadata JSONB`, `created_at`.
- `outbox_events` nếu dùng RabbitMQ production:
  - `id`, `topic`, `aggregate_type`, `aggregate_id`, `payload JSONB`, `status`, `attempt`, `available_at`, timestamps.
  - Ghi cùng transaction với document revision; dispatcher publish rồi đánh dấu sent. Đây là phần hoàn thiện giới hạn đã nêu trong ADR-0005.

### Database invariants

- Không có chunk nào thiếu `project_id` hoặc trỏ sang document project khác; enforce FK/composite FK khi khả thi và luôn test repository.
- Published version/accepted CR không cho sửa nội dung; thao tác thay thế phải tạo revision/scope mới.
- Chỉ revision `ready` được retrieval.
- Xóa document là soft delete + loại khỏi retrieval ngay; dọn object/chunk vật lý qua job riêng, có audit và retry.
- Transaction tạo upload metadata không được giả vờ thành công nếu outbox/job chưa được ghi.
- Embedding phải lưu model/version; query chỉ so vector cùng embedding space.

## API And State Contracts

Mọi response REST dùng ISC envelope. DTO dùng `snake_case`; timestamp UTC RFC3339; ID là UUID string.

### I. Đăng nhập

| Method | Route | Auth | Contract |
|---|---|---|---|
| POST | `/public/api/v1/auth/login` | No | `{email,password}` → user summary, `access_token`, `refresh_token`, expiry |
| POST | `/public/api/v1/auth/refresh` | No | refresh token → rotated access/refresh pair |
| POST | `/internal/api/v1/auth/logout` | Bearer | revoke current/all session theo body |
| GET | `/internal/api/v1/auth/me` | Bearer | actor profile và roles toàn cục |

- Thêm `jti`, `token_type` và session ID vào JWT claims; access token không được chấp nhận ở refresh endpoint.
- Login so sánh bcrypt hash, kiểm tra user active/locked và dùng thông báo chung để tránh dò email.
- Rate limit login theo IP hash + email normalized; không log password/token.
- Refresh token là random opaque secret, không phải access JWT tái sử dụng; chỉ hash được lưu.

### II. Tạo dự án

| Method | Route | Contract |
|---|---|---|
| POST | `/internal/api/v1/projects` | `{code,name,description}` → project; actor trở thành owner |
| PUT | `/internal/api/v1/projects/{project_id}` | update metadata với optimistic `version` |
| POST | `/internal/api/v1/projects/{project_id}/members` | owner thêm/sửa member role |
| DELETE | `/internal/api/v1/projects/{project_id}/members/{user_id}` | owner gỡ member; không được gỡ owner cuối |

- Tạo project và owner membership trong một transaction.
- Khi RAGFlow được bật, `POST /projects` phải tạo ngay một dataset riêng trên RAGFlow và lưu reference vào `projects.ragflow_dataset_id`; không dùng một dataset chung cho nhiều project.
- Tên dataset kỹ thuật có dạng `{APP_RAGFLOW_DATASET_PREFIX}_{project_uuid_without_hyphens}` để tránh đụng tên giữa local/dev/staging/prod hoặc giữa nhiều installation. Tên hiển thị của project vẫn nằm trong PostgreSQL.
- Luồng đồng bộ v1: kiểm tra code local → tạo dataset RAGFlow → transaction tạo project + owner membership + audit. Nếu transaction local thất bại, service gọi xóa dataset vừa tạo để bù trừ; API chỉ trả `201` khi mapping hai phía đã sẵn sàng.
- Response không lộ `ragflow_dataset_id` hoặc API key; chỉ trả `ragflow_sync_status=ready`. PostgreSQL vẫn là source of truth cho project, version, membership và ACL; RAGFlow chỉ giữ knowledge dataset/document/chunk.
- Nếu RAGFlow tắt, timeout, sai API key hoặc không có quyền tạo dataset, request tạo project thất bại bằng technical error; không tự tạo project local ở trạng thái thành công giả.
- Không tự tạo version “latest” rỗng; client hoặc seed tạo version draft rõ ràng.

### III. Danh sách dự án

| Method | Route | Contract |
|---|---|---|
| GET | `/internal/api/v1/projects` | filter `q,status,role`, pagination, chỉ dự án actor là member |
| GET | `/internal/api/v1/projects/{project_id}` | project summary, member role, latest published version, document counts |
| GET | `/internal/api/v1/projects/{project_id}/versions` | timeline version |
| POST | `/internal/api/v1/projects/{project_id}/versions` | owner/editor tạo draft version |
| PATCH | `/internal/api/v1/projects/{project_id}/versions/{id}/status` | state transition có optimistic lock |
| GET/POST | `/internal/api/v1/projects/{project_id}/change-requests` | list/tạo CR |
| GET/PATCH | `/internal/api/v1/projects/{project_id}/change-requests/{id}` | detail/state transition CR |

- List dùng shared pagination và stable sort `(updated_at,id)` hoặc `(created_at,id)`.
- Version timeline trả `sequence_no`; không suy ra thứ tự từ label.
- Trạng thái triển khai 2026-08-20: `POST/GET .../versions` đã có; version mới luôn là `draft`, owner/editor được tạo, mọi member được xem, `sequence_no` được cấp tuần tự dưới row lock của project và có audit `project_version.created`.
- Version chỉ là scope nghiệp vụ trong PostgreSQL và dùng chung `ragflow_dataset_id` của project; không tạo dataset RAGFlow mới cho từng version.
- `PATCH .../versions/{id}/status` chưa triển khai cho đến khi chốt P0 về lifecycle và định nghĩa `latest`.

### IV. Tra cứu thông tin

| Method | Route | Contract |
|---|---|---|
| POST | `/internal/api/v1/projects/{project_id}/search` | retrieval-only, trả chunks/scores/citations để debug/UI search |
| POST | `/internal/api/v1/projects/{project_id}/conversations` | tạo conversation, optional explicit scope |
| GET | `/internal/api/v1/projects/{project_id}/conversations` | list conversation của chính actor |
| GET | `/internal/api/v1/projects/{project_id}/conversations/{id}` | messages + citations |
| POST | `/internal/api/v1/projects/{project_id}/conversations/{id}/messages` | `{question,scope?}` → answer, intent, resolved_scope, citations |

Request scope có thể là:

```json
{
  "mode": "auto|latest|versions|change_requests|all",
  "version_ids": [],
  "change_request_ids": []
}
```

Response answer tối thiểu:

```json
{
  "answer": "... [S1] ...",
  "intent": "latest_state",
  "resolved_scope": {"mode":"latest","version_label":"v2.1"},
  "citations": [{
    "key":"S1",
    "document_id":"uuid",
    "document_revision_id":"uuid",
    "document_name":"requirements.md",
    "scope_type":"version",
    "scope_label":"v2.1",
    "line_start":42,
    "line_end":58,
    "page_start":null,
    "page_end":null,
    "excerpt":"...",
    "source_url":"/internal/api/v1/projects/.../documents/.../view?revision_id=...&line=42"
  }],
  "grounded": true
}
```

- `source_url` là application URL hoặc short-lived presigned URL; không lưu presigned URL hết hạn vào DB.
- Server chỉ chấp nhận citation key thuộc context đã retrieval. Citation do model tự bịa phải bị loại; nếu câu trả lời phụ thuộc citation không hợp lệ thì regenerate một lần hoặc trả abstention.
- `quoted_text` giới hạn ngắn để tránh trả toàn bộ tài liệu qua citation.

### V. Tải tài liệu

| Method | Route | Contract |
|---|---|---|
| POST | `/internal/api/v1/projects/{project_id}/documents/uploads` | multipart nhỏ hoặc tạo upload session; metadata bắt buộc chỉ rõ version/CR |
| POST | `/internal/api/v1/projects/{project_id}/documents/uploads/presign` | khuyến nghị cho file lớn: trả object key + presigned PUT |
| POST | `/internal/api/v1/projects/{project_id}/documents/uploads/{upload_id}/complete` | verify object size/hash, tạo revision và enqueue ingestion |
| GET | `/internal/api/v1/projects/{project_id}/documents/{id}/revisions/{revision_id}/status` | polling trạng thái ingestion |

- Validate extension, MIME thực tế, size, empty file, SHA-256 và object key do server tạo; không tin filename/path từ client.
- Tránh `[]byte` cho file lớn: bổ sung `ObjectStore.PutReader(ctx,key,io.Reader,size,contentType)` hoặc presigned upload.
- Object key dạng không đoán được: `projects/{projectID}/documents/{documentID}/revisions/{revisionID}/{safeName}`.
- Upload hoàn tất trả `202` trong data/envelope semantics của repo nếu đội chấp nhận HTTP status này; nếu chuẩn ISC bắt buộc status khác thì ghi contract trước khi code.

### VI. Quản lý tài liệu

| Method | Route | Contract |
|---|---|---|
| GET | `/internal/api/v1/projects/{project_id}/documents` | filter scope/status/type/q, pagination |
| GET | `/internal/api/v1/projects/{project_id}/documents/{id}` | logical document + revisions + ingestion status |
| PATCH | `/internal/api/v1/projects/{project_id}/documents/{id}` | đổi title/description với optimistic lock |
| POST | `/internal/api/v1/projects/{project_id}/documents/{id}/revisions` | upload revision mới vào scope hợp lệ |
| POST | `/internal/api/v1/projects/{project_id}/documents/{id}/revisions/{revision_id}/retry` | retry failed ingestion, idempotent |
| GET | `/internal/api/v1/projects/{project_id}/documents/{id}/revisions/{revision_id}/view` | canonical text/metadata hoặc redirect/presign tới original |
| GET | `/internal/api/v1/projects/{project_id}/documents/{id}/revisions/{revision_id}/download` | short-lived presigned GET |
| DELETE | `/internal/api/v1/projects/{project_id}/documents/{id}` | soft delete, loại khỏi retrieval, enqueue cleanup |

- Không cho retry revision đang running/ready trừ thao tác re-index có quyền riêng.
- Re-index tạo generation mới hoặc transactionally replace chunks; query không được thấy nửa bộ chunk cũ/nửa bộ mới.
- UI status nên hiển thị `uploaded`, `queued`, `processing`, `ready`, `failed`, kèm lỗi đã sanitize và số lần retry.

### VII. Cổng tích hợp (MCP)

- Dùng `github.com/modelcontextprotocol/go-sdk/mcp` bản stable đã pin.
- HTTP transport: endpoint riêng `/mcp` hoặc server port riêng, Streamable HTTP; middleware phải xác thực bearer token/API token và đưa actor vào context.
- Stdio transport: `cmd/mcp` dành cho client local; token/endpoint lấy từ ENV, không hardcode credential.
- MCP handler không query DB trực tiếp. Nó gọi `project`, `document` và `chat/search` usecase để dùng lại ACL, intent và citation.
- Resources đề xuất:
  - `project://{project_id}`: project metadata mà actor được xem.
  - `project://{project_id}/versions/{version_id}`: version metadata.
  - `project://{project_id}/documents/{document_id}/revisions/{revision_id}`: canonical content có line markers, chỉ khi actor có quyền.
- Tools v1:
  - `list_projects(q?,page?)`.
  - `list_project_versions(project_id)`.
  - `search_project(project_id,query,scope?,limit?)`.
  - `ask_project(project_id,question,scope?,conversation_id?)`.
  - `get_document_source(project_id,document_id,revision_id,line_start?,line_end?)`.
  - `compare_project_versions(project_id,question,version_ids?,change_request_ids?)`.
- Không expose upload/delete qua MCP trong v1; mutation tools cần threat model, idempotency key, confirmation và audit riêng.
- Tool output trả structured content gồm answer/citations; lỗi không được lộ SQL, object key nội bộ, prompt hoặc stack trace.
- Rate limit MCP theo principal + tool; giới hạn `limit`, excerpt length và tổng context để ngăn data exfiltration.

## RAG And AI Design

### LocalAI boundary

Tạo port thuần ở `internal/common/port` hoặc module-specific port:

- `EmbeddingClient.Embed(ctx,texts,model) ([][]float32,error)`.
- `ChatClient.Complete(ctx,request) (response,error)` với structured JSON mode nếu model/backend hỗ trợ.
- Implementation HTTP đặt ở `internal/infrastructure/ai/localai`; dùng `net/http` client được inject, timeout từ config, không dùng global default client.
- Gọi `/v1/embeddings` cho embedding và `/v1/chat/completions` cho intent/rewrite/answer; health checker có timeout ngắn.
- Log model, token estimate, latency, prompt version và status; không log nguyên tài liệu/câu hỏi ở production mặc định.
- Chat model và embedding model là hai config riêng. Qwen nhỏ dùng sinh/hiểu query; embedding model cần đa ngôn ngữ Việt/Anh, dimension cố định và được benchmark trên dữ liệu thật.

### Ingestion pipeline

1. API xác thực actor/project/editor và scope version/CR.
2. Stream file vào MinIO, tính SHA-256 trong lúc upload; verify size/type.
3. Transaction tạo document/revision + ingestion job + outbox event `document.ingestion.requested`.
4. Dispatcher publish event; consumer worker nhận và lock job idempotently.
5. Parser tạo canonical UTF-8 text, line mapping và page mapping; lưu canonical artifact.
6. Deterministic chunker chia theo heading/paragraph/list/table trước; dùng target khoảng 300-600 tokens và overlap khoảng 50-80 tokens, nhưng giá trị thật qua config/eval.
7. AI semantic boundary refinement chỉ là bước tùy chọn: không được làm mất dòng, đổi nội dung hoặc gộp hai scope; khi LocalAI lỗi phải fallback deterministic chunking.
8. Batch embedding qua LocalAI; validate vector count, finite values và đúng dimension.
9. Transaction ghi toàn bộ chunk generation, update revision `ready`; nếu lỗi, rollback generation và đánh dấu job retryable/failed.
10. Ack message chỉ sau khi DB commit; duplicate delivery phải no-op nhờ job/revision state và content hash.

### Canonical line mapping and citation

- Parser không chỉ trả plain string; trả block gồm text, heading path, original page, canonical `line_start/line_end`.
- Chuẩn hóa newline về `\n`, không trim/gộp dòng sau khi đã gán line number.
- Markdown/text dùng dòng gốc sau decode; PDF dùng dòng của extracted canonical text và page number gốc.
- Viewer đọc canonical artifact, nhảy đến line và highlight range; nút “xem bản gốc” dùng presigned MinIO URL hoặc external source URL.
- Khi tạo citation, server snapshot locator từ chunk; model chỉ chọn `[S1]`, `[S2]`, không tự tạo URL/line number.

### Query understanding and scope resolution

Áp dụng thứ tự deterministic-first, LLM-second:

1. Validate explicit `scope` trong request và quyền actor.
2. Parse entity chắc chắn từ query: version label, CR code, tên tài liệu; map sang ID trong đúng project.
3. LocalAI classifier trả JSON schema với intent:
   - `latest_state`: “chức năng X hoạt động thế nào”, “hiện tại”, không nêu mốc → latest published version.
   - `evolution`: “đã thay đổi như thế nào qua…”, “so sánh”, “lịch sử” → timeline versions/CRs.
   - `specific_revision`: nêu rõ version/CR → chỉ scope đó.
   - `broad_project`: câu hỏi tổng quan không phụ thuộc thời gian → latest trước, mở rộng có kiểm soát nếu thiếu bằng chứng.
4. Nếu classifier output sai schema/timeout, dùng rule fallback; không để lỗi AI biến thành truy vấn all-project/all-version.
5. Resolve scope thành ID bất biến và lưu vào message để hội thoại sau có thể dùng ngữ cảnh.

### Query rewrite and hybrid retrieval

- Rewrite giữ nguyên identifier, version, CR code và thuật ngữ nghiệp vụ; tạo tối đa vài biến thể ngắn. Luôn giữ original query trong tập truy hồi.
- Embed original + rewritten queries theo batch; không cho model rewrite tự thay đổi scope.
- PostgreSQL semantic retrieval dùng cosine distance trên pgvector với filter project/scope/revision ready.
- Lexical retrieval dùng `websearch_to_tsquery`/cấu hình text search phù hợp; vì tài liệu có tiếng Việt, benchmark tokenizer/unaccent. Nếu PostgreSQL FTS tiếng Việt không đủ tốt, phase sau bổ sung trigram/BM25 service qua ADR, không âm thầm bật Elasticsearch.
- Fuse hai ranking bằng Reciprocal Rank Fusion (RRF) hoặc weighted score đã config; log component scores để debug.
- Deduplicate chunk overlap bằng `content_hash`/document+line range; giới hạn số chunk mỗi document để tăng đa dạng nguồn.
- Với `evolution`, retrieval chạy theo từng scope và yêu cầu coverage tối thiểu mỗi version/CR; sau đó sắp timeline. Không dùng global top-K vì version nhiều tài liệu có thể lấn át version khác.
- Optional lightweight rerank có thể dùng LocalAI, nhưng phải có timeout/fallback về fused ranking.

### Answer generation

- Prompt system cấm dùng kiến thức ngoài context, yêu cầu trả đúng ngôn ngữ người hỏi và citation key sau mỗi claim.
- `latest_state`: prompt nói rõ resolved latest label và cấm lấy draft/open CR.
- `evolution`: context nhóm theo timeline; output gồm baseline, thay đổi ở từng mốc và trạng thái cuối.
- Giới hạn context theo token budget của model nhỏ; ưu tiên evidence diversity và query relevance.
- Grounding validator kiểm tra citation tồn tại, scope đúng và claim quan trọng có citation. Không đạt: regenerate một lần với lỗi cụ thể, sau đó abstain.
- Không lưu chain-of-thought. Chỉ lưu intent, resolved scope, prompt version, retrieval IDs/scores cần audit và final answer.

### Evaluation targets

- Tạo fixture project có ít nhất 3 versions và 2 CRs, trong đó cùng một chức năng thay đổi qua từng mốc.
- Dataset tối thiểu gồm câu latest, explicit version, evolution, follow-up, câu không có đáp án và câu cố thử lấy dữ liệu project khác.
- Metrics: scope accuracy, retrieval recall@K, citation precision, citation locator validity, grounded answer rate, abstention correctness, latency p50/p95 và peak RAM.
- Không đặt ngưỡng production tùy ý. Baseline đo trước, product/tech lead duyệt threshold; CI có smoke eval deterministic, full LLM eval chạy riêng để tránh flaky.

## Implementation Plan

### Phase 0 - Contract and ADR gate

1. Tạo `docs/architecture/ADR-0007-go-localai-rag.md` ghi rõ phần ADR-0006 bị thay thế: Go worker, PostgreSQL hybrid-only v1, LocalAI, model separation, citation line semantics và MCP transport.
2. Chốt P0: model IDs/dimension, latest semantics, XOR version/CR, project ACL.
3. Viết OpenAPI trước cho auth/project/version/CR/document/chat; thêm error catalogue.
4. Chốt event schemas có `schema_version`, `event_id`, `revision_id`, `project_id`, `trace_id`.

### Phase 1 - Authentication and project boundaries

1. Hoàn thiện `auth` vertical slice và refresh session migration.
2. Mở rộng JWT claims/manager nhưng giữ API verify/sign cũ hoặc sửa tất cả caller/test cùng lúc.
3. Tạo `project` slice gồm project, member, version và change request.
4. Tạo reusable project authorization service/port; không đặt business ACL trong Gin middleware đơn thuần.
5. Thêm routes/module wiring, OpenAPI, unit/integration tests.
6. Trong create-project usecase, tạo dataset RAGFlow theo prefix môi trường, persist mapping trong cùng lifecycle với owner membership và bù trừ remote dataset nếu transaction local rollback.

### Phase 2 - Document upload and management

1. Chuyển scaffold `file` thành module `document` hoặc giữ `file` chỉ làm storage adapter; ưu tiên tên domain `document` để tránh nhầm file thuần.
2. Bổ sung streaming/presigned methods vào ObjectStore và MinIO implementation.
3. Tạo migrations document/revision/job/outbox/audit.
4. Hiện thực upload complete, dedup, status, list/detail/download/retry/delete.
5. Bảo đảm mọi operation nhận project scope và gọi project authorizer.

### Phase 3 - Go ingestion worker

1. Tạo consumer abstraction và RabbitMQ implementation có queue durable, QoS, retry/dead-letter, reconnect.
2. Tạo `cmd/worker/main.go`, bootstrap worker và graceful shutdown.
3. Tạo parser registry; triển khai text/Markdown/PDF text-layer.
4. Tạo deterministic chunker, canonical line mapping và tests golden.
5. Tạo LocalAI embedding adapter, dimension validation và batch retry.
6. Ghi chunk generation atomically, chuyển trạng thái job/revision idempotently.

### Phase 4 - Search and conversational RAG

1. Tạo `knowledge` hoặc `search` slice cho hybrid repository; SQL phải filter quyền/scope trước ranking.
2. Tạo `chat` slice cho conversation/message, intent resolver, query rewriter, retrieval orchestrator, prompt builder, grounding/citation validator.
3. Tạo LocalAI chat adapter và prompt templates có version trong `internal/module/chat/prompt` hoặc embedded files không global mutable.
4. Hiện thực search API trước để kiểm chứng retrieval; sau đó ask API.
5. Thêm metrics/traces không chứa nội dung nhạy cảm.

### Phase 5 - MCP

1. Pin official Go SDK stable và viết ADR/compatibility note về protocol versions/client đã test.
2. Tạo `internal/module/mcpserver` làm delivery adapter gọi usecases; không phải một business domain mới.
3. Hiện thực resources/tools read-only, JSON schema chặt, auth, rate limit, audit.
4. Tạo `cmd/mcp` cho stdio nếu cần; Streamable HTTP dùng port/route riêng để không phá ISC envelope REST.
5. Chạy SDK/conformance checks phù hợp và integration test với một MCP client thật.

### Phase 6 - Hardening and rollout

1. RAG eval, load test, cross-project leakage test, object orphan cleanup và restore/re-index drill.
2. Dashboard: queue depth, job age/failure, LocalAI latency/error, retrieval latency, grounded/abstain rate.
3. Feature flags theo module: ingestion, chat, MCP; rollout internal project trước.
4. Backup PostgreSQL + MinIO phải cùng chiến lược consistency; document runbook model upgrade/re-index.

### Must Change

- `go.mod`, `go.sum`: parser/MCP dependencies đã chọn và pin.
- `internal/config/*`, `configs/config.*.yaml`, `.env.example`: LocalAI, ingestion, chunk, upload, MCP và model configs.
- `internal/bootstrap/infra.go`, `modules.go`, `router.go`, `app.go`: LocalAI/consumer/module/worker/MCP wiring.
- `internal/common/port/port.go` hoặc port file tách nhỏ: streaming object store, AI clients, consumer/outbox/authorizer nếu dùng chung ít nhất hai module.
- `internal/infrastructure/storage/minio/*`, `mq/rabbitmq/*`, `ai/localai/*`.
- `internal/module/auth/*`, `project/*`, `document/*`, `search/*`, `chat/*`, `mcpserver/*`.
- `pkg/jwt/*`: token claims/type/jti/session support.
- `migrations/*`, `docs/api/openapi/*`, `ERROR_CODES.md`, ADR mới, README/runbook.
- `deployments/compose/docker-compose.yml`: LocalAI service/profile, model volumes/healthcheck và worker.
- `cmd/worker/*`; optional `cmd/mcp/*`.
- Unit/integration/e2e/eval tests tương ứng.

### Inspect Only

- `internal/module/user/*`: đọc và reuse pattern; chỉ sửa interface nhỏ nếu auth thật sự cần và có test regression.
- `internal/common/response/*`, `internal/middleware/errorhandler.go`: giữ chuẩn envelope/error; chỉ thêm test nếu route mới phơi ra case mới.
- `migrations/000001*`, `000002*`: không sửa.
- `docs/swagger/docs.go`: generated output, chỉ cập nhật bằng `make swagger`.
- `internal/infrastructure/database/postgres/*`: reuse transaction/context/error mapping; chỉ mở rộng generic vector support khi cần.

### Out Of Scope

- Refactor module `user` không liên quan.
- Thay Gin/GORM/Redis/RabbitMQ/MinIO.
- Tích hợp cloud LLM hoặc gửi tài liệu ra internet.
- UI frontend, mobile app.
- Fine-tuning Qwen, tự huấn luyện embedding.
- MCP write tools và arbitrary SQL/file access.

## Files And Areas To Touch

| Path/module | Change type | What/why | Preserve/risk |
|---|---|---|---|
| `docs/architecture/ADR-0007-go-localai-rag.md` | new | Ghi quyết định thay thế ADR-0006 có kiểm soát | ADR-0006 vẫn là lịch sử; nêu chính xác phần superseded |
| `migrations/000003...` trở đi | new | Auth sessions, project ACL/version/CR, document/chunk/chat/audit/outbox | Có down migration; model dimension là gate |
| `internal/module/auth/` | modify | Login/refresh/logout/me | Giữ dev-token local; tránh account enumeration |
| `internal/module/project/` | new | Project/member/version/CR | ACL và state transition là domain invariant |
| `internal/module/document/` | new | Upload, metadata, revision, management | Không giữ file lớn trong RAM; object orphan handling |
| `internal/module/search/` | new | Scope-filtered hybrid retrieval | Cross-project leakage là rủi ro P0 |
| `internal/module/chat/` | new | Conversation, intent, rewrite, answer, citation | Không lưu chain-of-thought; validate citation |
| `internal/module/mcpserver/` | new | MCP delivery adapter | Reuse usecase/ACL; read-only v1 |
| `internal/infrastructure/ai/localai/` | new | OpenAI-compatible HTTP adapters + health | Timeout, model unload/503, dimension mismatch |
| `internal/infrastructure/mq/rabbitmq/` | modify | Consumer/DLQ/reconnect | Publisher behavior hiện tại không được regression |
| `internal/infrastructure/storage/minio/` | modify | streaming/presign PUT/head | Không phá ObjectStore callers hiện tại |
| `internal/common/port/` | modify/new files | Shared infrastructure contracts | Chỉ đưa contract thực sự dùng chung; tránh god interface |
| `internal/config/*`, `configs/*`, `.env.example` | modify | Cấu hình không secret | Fail-fast; local defaults không chảy sang production |
| `internal/bootstrap/*` | modify | Constructor wiring/modules/routes/checkers | Không DI framework/global |
| `pkg/jwt/*` | modify | jti/token type/session claims | Regression test verify token cũ nếu cần compatibility |
| `cmd/worker/` | new | Ingestion/outbox worker | Graceful shutdown, idempotent |
| `cmd/mcp/` | new/optional | Stdio MCP bridge | Credential chỉ ENV |
| `deployments/compose/docker-compose.yml` | modify | LocalAI + worker profiles/volumes | API port collision; CPU/RAM limits |
| `docs/api/openapi/` | new/modify | Contract REST | Swagger generated, không hand-edit docs.go |
| `ERROR_CODES.md`, `internal/common/errcode/*` | modify | Mã lỗi feature mới | Business/technical split chuẩn ISC |
| `test/integration/`, module `_test.go`, `test/eval/` | new/modify | Auth, ACL, ingestion, RAG, MCP | Fixture không chứa tài liệu nội bộ |

## Reuse Existing Methods / Patterns

- Copy cấu trúc và naming từ `internal/module/user`, không copy nghiệp vụ user.
- Dùng `port.TxManager.Do` cho aggregate transaction; repository tự lấy transaction từ context.
- Dùng `port.Cache` cho rate limit/session cache ngắn hạn, nhưng nguồn sự thật refresh revocation là PostgreSQL nếu cần bền qua restart.
- Dùng `port.Publisher` cho event và mở rộng outbox/consumer theo interface nhỏ.
- Dùng `port.ObjectStore`, MinIO health checker và presigned GET hiện có.
- Dùng `contextx.Actor`, mở rộng bằng helper project authorization thay vì nhét project role vào JWT vì membership có thể đổi ngay.
- Dùng shared `pagination.Query/Meta`, `validatorx`, `response` và `apperr`.
- Dùng `middleware.Auth` và `RequireRoles` cho global role; project role phải kiểm tra từ DB/cache ở usecase.
- Dùng config Viper + ENV prefix `APP_`; mọi timeout/limit/model ID phải config được.
- Dùng OpenTelemetry trace/request ID xuyên API → outbox event → worker → LocalAI.

## Data / API / Contract Notes

### Suggested config additions

- `APP_RAGFLOW_ENABLED`, `APP_RAGFLOW_BASE_URL`, `APP_RAGFLOW_API_KEY`: bật và xác thực RAGFlow; API key thật chỉ nằm trong `.env`/secret manager.
- `APP_RAGFLOW_DATASET_PREFIX`: prefix phân vùng dataset theo installation/môi trường, ví dụ `docs_hub_local`, `docs_hub_staging`.

```yaml
localai:
  enabled: true
  base_url: http://127.0.0.1:8081
  chat_model: qwen-local
  embedding_model: multilingual-embedding-local
  embedding_dimension: 384
  request_timeout: 60s

rag:
  chunk_target_tokens: 450
  chunk_overlap_tokens: 64
  retrieval_limit: 12
  per_scope_limit: 5
  rrf_k: 60
  max_context_tokens: 6000
  prompt_version: v1

upload:
  max_bytes: 52428800
  allowed_media_types: [text/plain, text/markdown, application/pdf]
  presign_ttl: 15m

mcp:
  enabled: false
  address: 127.0.0.1:8090
```

Các giá trị trên chỉ là development defaults. `embedding_dimension: 384` là ví dụ và vẫn bị chặn cho tới khi chọn model.

### Suggested event envelope

```json
{
  "event_id":"uuid",
  "schema_version":1,
  "type":"document.ingestion.requested",
  "occurred_at":"RFC3339",
  "project_id":"uuid",
  "document_revision_id":"uuid",
  "trace_id":"..."
}
```

- Consumer không tin payload về quyền hay object key; load revision từ DB bằng ID.
- Event schema version không đồng nhất với document/version business version.

### Suggested error codes

- Business: `INVALID_CREDENTIALS`, `SESSION_REVOKED`, `PROJECT_CODE_EXISTS`, `PROJECT_MEMBER_EXISTS`, `PROJECT_LAST_OWNER`, `VERSION_LABEL_EXISTS`, `INVALID_VERSION_STATE`, `CHANGE_REQUEST_CODE_EXISTS`, `INVALID_CHANGE_REQUEST_STATE`, `DOCUMENT_DUPLICATE`, `DOCUMENT_NOT_READY`, `INGESTION_ALREADY_RUNNING`, `UNSUPPORTED_FILE_TYPE`, `FILE_TOO_LARGE`, `RAG_NO_EVIDENCE`.
- Technical: dùng/định nghĩa catalogue phù hợp cho `PROJECT_404`, `DOCUMENT_404`, `AI_502`, `AI_504`, `STORAGE_502`, `MQ_502`, `DB_500`.
- Quyền thiếu là `AUTH_403`; resource không tồn tại hoặc không được phép xem có thể trả cùng not-found policy để giảm dò ID, cần thống nhất toàn API.

## Error Handling And Edge Cases

- Sai password, user không tồn tại, user locked: thông điệp login không tiết lộ email tồn tại; locked có thể là business code nếu policy cho phép.
- Refresh token reuse sau rotate: revoke token family/session, yêu cầu login lại và audit security event.
- Hai request tạo cùng project/version/CR: unique index là nguồn bảo vệ cuối; map SQLSTATE `23505` sang BusinessError.
- Owner cuối bị gỡ/xóa: chặn ở transaction với row lock.
- Upload client ngắt giữa chừng: multipart/presign session hết hạn và cleanup object không có completed revision.
- MIME/extension giả, zip bomb, PDF mã hóa, file rỗng, binary trong text, invalid UTF-8: reject/sanitize; không retry vô hạn lỗi permanent.
- Duplicate MQ delivery/worker crash sau commit trước ack: job idempotency trả success rồi ack.
- LocalAI unavailable/OOM/model loading 503: exponential backoff có jitter, circuit/queue backpressure; query trả TechnicalError rõ ràng, ingestion giữ trạng thái retryable.
- Vector dimension/NaN mismatch: fail job trước DB insert và metric critical.
- Re-index đang diễn ra: query tiếp tục dùng active generation cũ cho đến atomic switch.
- Version không có tài liệu: evolution answer nêu rõ mốc không đủ nguồn, không giả định “không thay đổi”.
- Câu hỏi nêu version/CR không tồn tại: hỏi lại hoặc BusinessError có candidates; không fallback latest âm thầm.
- Prompt injection trong tài liệu: context được đánh dấu là dữ liệu không phải instruction; không cho model gọi tool từ text tài liệu.
- Citation tới tài liệu vừa bị archive/xóa hoặc actor mất quyền: source endpoint kiểm tra lại ACL và trả forbidden/not-found; lịch sử answer vẫn giữ metadata snapshot nhưng không lộ excerpt nếu policy yêu cầu.
- Cross-project conversation ID hoặc document ID: repository query phải chứa cả `project_id` và owner/user scope; test IDOR bắt buộc.
- Câu hỏi follow-up mơ hồ: dùng resolved scope của conversation gần nhất; nếu có xung đột entity thì yêu cầu người dùng làm rõ.
- Context vượt model window: token budgeter cắt theo evidence unit, không cắt giữa citation/chunk làm sai locator.
- MCP client ngắt request dài: propagate context cancellation xuống chat/retrieval/LocalAI khi SDK/transport hỗ trợ.

## Backwards Compatibility Rules

- Không đổi/remove field endpoint hiện có trong cùng `/v1`; field mới optional hoặc route mới.
- Không sửa migration đã chạy; mọi schema correction bằng migration mới.
- Model/prompt/chunker/parser thay đổi phải tăng version metadata; document cũ vẫn query bằng active compatible generation hoặc được re-index có kiểm soát.
- Event consumer hỗ trợ ít nhất schema version đang lưu trong outbox/queue lúc deploy; deploy consumer trước producer khi thêm event version.
- MCP tool name/input schema được coi là public contract; thay đổi breaking tạo tool version mới hoặc duy trì adapter.
- Citation source URL là application route ổn định; presigned URL chỉ tạo theo request, không serialize lâu dài.

## Clarifying Questions

Các câu này không chặn skeleton/module cơ bản nhưng phải được trả lời trước phần tương ứng:

1. “Version mới nhất” có bao gồm accepted change request chưa được phát hành không, hay chỉ version `published`?
2. Model cụ thể chạy trong LocalAI là gì (chat model, embedding model, quantization), máy mục tiêu có bao nhiêu RAM/VRAM và embedding dimension bao nhiêu?
3. V1 cần các định dạng nào ngoài TXT/Markdown/PDF text-layer, và với PDF scan có bắt buộc OCR không?
4. Dự án là single organization với project ACL hay multi-tenant; owner/editor/viewer có đủ không?
5. MCP client mục tiêu là Codex, Claude Desktop, VS Code hay hệ nội bộ; cần Streamable HTTP, stdio hay cả hai?

## QA Focus

### Functional

- Login/refresh rotation/logout/me hoạt động; token revoked không dùng lại được.
- Tạo project đồng thời tạo đúng một dataset RAGFlow, lưu mapping local, tạo owner membership và không expose remote ID; list/get chỉ thấy project có membership, permission matrix đúng.
- Version/CR state transition đúng và latest resolution dùng sequence/status, không dùng label lexical.
- Upload vào đúng scope, worker tạo canonical text/chunks/vector và status chuyển đúng.
- Search trả đúng project/scope và locator có thể mở.
- Câu latest chỉ dùng latest published; câu evolution bao phủ/sắp đúng từng mốc; explicit version không rò mốc khác.
- Answer không có nguồn trả abstention; citation key/line/page/document/link hợp lệ.
- MCP tools trả cùng kết quả/quyền như REST usecase tương ứng.

### Regression

- User CRUD tests, ISC envelope golden test, middleware error classification và dev-token safety vẫn pass.
- PostgreSQL/Redis/RabbitMQ/MinIO readiness hiện có vẫn hoạt động khi LocalAI/MCP disabled.
- `make run`, migration/seed và Swagger flow không bị phá.

### Edge Cases

- Parallel create/update conflict, last-owner removal, IDOR/cross-project IDs.
- RAGFlow create lỗi không để lại project local; transaction local lỗi phải thử xóa dataset remote vừa tạo; prefix môi trường không dùng lại tên dataset cũ của tenant khác.
- Duplicate upload/event, worker restart, poison file, retry exhaustion, DLQ.
- Unicode tiếng Việt, mixed Vietnamese/English, version labels gần giống nhau.
- Empty/huge question, prompt injection, no evidence, LocalAI timeout/OOM.
- Deleted/archived document citations và expiring source links.
- Evolution với mốc không có document hoặc contradictory sources.

### Verification

Chạy theo thứ tự; các lệnh cần Docker được ghi rõ:

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
make lint
make test
make test-integration
make swagger
make ci
```

Kiểm tra bổ sung dự kiến:

```bash
go test -tags=integration ./internal/module/...
go test -tags=integration ./test/integration/...
go test ./test/eval/... -run TestRAGSmoke
go test -race ./internal/module/project/... ./internal/module/document/... ./internal/module/chat/...
```

- `make test-integration`, LocalAI smoke và MCP conformance cần Docker/model đã tải; nếu môi trường CI không có model, dùng fake LocalAI HTTP server cho contract test và chạy eval thật ở job riêng.
- Manual QA: upload fixture, mở citation ở đúng line/page, đổi membership khi conversation đang tồn tại, hỏi cùng câu ở latest/specific/evolution, kiểm tra MCP từ client mục tiêu.
- Security QA: IDOR matrix, malicious filename, oversized upload, prompt injection fixture, bearer/API token leakage trong log, rate limit và cross-project vector query.
- Performance QA trên máy mục tiêu: ingestion throughput, peak RAM, LocalAI cold/warm latency, p95 ask latency và queue backpressure.

## Acceptance Criteria

1. Người dùng hợp lệ đăng nhập, refresh/logout an toàn; user không hợp lệ không nhận token.
2. Người dùng chỉ list/xem/thao tác dự án theo role; test tự động chứng minh không truy cập chéo project.
3. Project tạo được version và CR có timeline/state hợp lệ.
4. TXT/Markdown/PDF text-layer được upload vào đúng version hoặc CR, xử lý bất đồng bộ và đạt trạng thái ready/failed rõ ràng.
5. Mỗi chunk có project/scope/document locator, canonical line range và embedding hợp lệ.
6. Câu hỏi không chỉ rõ thời gian được resolve về latest published version đã xác nhận.
7. Câu hỏi evolution trả thay đổi theo thứ tự từng version/CR và không bỏ mốc chỉ vì global top-K.
8. Mỗi claim quan trọng có citation server-validated; link mở đúng tài liệu và highlight được line/page nếu UI hỗ trợ.
9. Không đủ bằng chứng thì hệ thống từ chối có kiểm soát, không hallucinate câu trả lời.
10. LocalAI và embedding model cấu hình được; không có nội dung tài liệu gửi ra dịch vụ cloud.
11. Worker idempotent trước duplicate event/restart và không để partial chunk generation được query.
12. MCP read/query tools hoạt động với client mục tiêu và áp cùng ACL/citation như REST.
13. Existing unit tests + lint pass; integration/eval/security cases quan trọng có kết quả được lưu trong CI/report.
14. Comment tiếng Việt giải thích các phần phức tạp/invariant/thuật toán; không comment dư thừa từng statement cơ bản.

## Definition Of Done

- [ ] Các quyết định P0 và ADR-0007 được duyệt.
- [ ] OpenAPI, data schema, event schema và MCP tool schemas được review trước implementation public contract.
- [ ] Tất cả migration up/down chạy được trên DB sạch và nâng cấp từ schema hiện tại.
- [ ] Auth, project ACL, version/CR, document management, worker, search/chat/citation và MCP v1 được triển khai theo phase.
- [ ] Không có code domain/usecase import framework/infrastructure; lint architecture pass.
- [ ] Không có secret/model binary/tài liệu nội bộ được commit.
- [ ] Unit, integration, IDOR/security, idempotency, citation và RAG smoke eval pass theo threshold đã duyệt.
- [ ] `make ci` pass; mọi check không chạy được được ghi lý do và owner xử lý.
- [ ] Health/readiness, metrics, tracing và structured log cho PostgreSQL/Redis/RabbitMQ/MinIO/LocalAI/worker đã có.
- [ ] Runbook có hướng dẫn startup máy yếu, tải model, migrate, retry/DLQ, re-index, backup/restore và rollback.
- [ ] MCP compatibility/conformance được kiểm tra với SDK stable và client mục tiêu.
- [ ] Swagger/README/ERROR_CODES/ADR được cập nhật; generated files được tạo bằng command chuẩn.
- [ ] Product owner xác nhận thủ công ba kịch bản: latest, explicit version và evolution qua version/CR.
