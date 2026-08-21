# Phân tích tích hợp RAGFlow qua API key

## 1. Mục tiêu

Tài liệu này phân tích các thay đổi cần thực hiện để `docs-hub-api` tích hợp với RAGFlow thông qua HTTP API và API key.

Mục tiêu của tích hợp:

- Giữ `docs-hub-api` làm API/BFF chịu trách nhiệm xác thực, phân quyền, metadata tài liệu và điều phối nghiệp vụ.
- Giữ PostgreSQL local làm nguồn dữ liệu chuẩn cho project, document, revision, version, change request và trạng thái đồng bộ RAGFlow.
- Dùng RAGFlow cho parse tài liệu, chunking, embedding, indexing, retrieval và có thể cả bước gọi LLM sinh câu trả lời.
- Không để frontend truy cập trực tiếp RAGFlow hoặc biết RAGFlow API key.
- Bảo đảm mọi thao tác ingestion và retrieval đều tuân thủ ACL theo project của `docs-hub-api`.
- Có khả năng retry, quan sát, đối soát và khôi phục khi RAGFlow tạm thời không khả dụng.

### 1.1. Trạng thái triển khai MVP ngày 2026-08-20

Đã triển khai trong repository:

- Nạp RAGFlow API key và cấu hình từ `.env`/environment với prefix `APP_RAGFLOW_`.
- Fail-fast khi bật RAGFlow nhưng thiếu base URL, API key hoặc timeout hợp lệ.
- Migration lưu `ragflow_dataset_id`, `ragflow_document_id`, sync status, thời điểm sync và lỗi đã sanitize.
- HTTP client cho health, dataset create/find/update/delete, document upload/find/status/delete, start/stop parsing và retrieval.
- Health checker và dependency injection cho API process.
- Ingestion worker chọn RAGFlow khi `ragflow.enabled=true`; LocalAI/pgvector vẫn là fallback khi tắt.
- Dataset được tạo/mapping theo project; document remote được mapping theo revision local.
- Poll trạng thái parse và chỉ chuyển revision thành `ready` khi RAGFlow hoàn tất.
- Cleanup document remote từ outbox khi document local bị soft-delete.
- Public internal retrieval endpoint bắt buộc project ACL và đúng một version/change-request scope.
- Retrieval resolve allowed revision IDs từ PostgreSQL rồi mới gửi RAGFlow document IDs.
- Citation được map và lọc lại bằng local project/revision mapping trước khi trả client.
- Unit tests cho config, RAGFlow HTTP client và retrieval ACL/citation filtering.

Chưa nằm trong MVP hiện tại:

- Chat assistant/completion và conversation persistence.
- Agent, Memory, audio, mind map, Search App, GraphRAG và RAPTOR.
- RAGFlow workspace commits làm version manager; PostgreSQL local vẫn giữ trách nhiệm này.
- UI/admin API sửa chunk trực tiếp.

## 2. Hiện trạng của repository

Repository hiện đã có một pipeline RAG nội bộ:

```text
Upload
  -> lưu file vào filesystem/MinIO
  -> lưu metadata vào PostgreSQL
  -> tạo ingestion job
  -> worker parse tài liệu
  -> chunk nội dung
  -> gọi LocalAI tạo embedding
  -> lưu chunk và vector vào PostgreSQL/pgvector
```

Các thành phần liên quan:

- `internal/module/document`: upload và quản lý metadata tài liệu.
- `internal/module/ingestion`: parse, chunk, embedding và ghi `document_chunks`.
- `internal/infrastructure/ai/localai`: client OpenAI-compatible embeddings.
- `internal/infrastructure/storage`: filesystem/MinIO object storage.
- `migrations/000003_create_documents.*.sql`: document, revision và ingestion job.
- `migrations/000004_create_document_chunks.*.sql`: chunk và pgvector.
- `docs/architecture/ADR-0006-rag-architecture.md`: quyết định kiến trúc RAG hiện tại.

RAGFlow cung cấp lại phần lớn trách nhiệm đang nằm trong `internal/module/ingestion`. Nếu chỉ thêm RAGFlow mà vẫn giữ pipeline hiện tại hoạt động song song, hệ thống sẽ có hai nguồn index độc lập:

- PostgreSQL/pgvector trong `docs-hub-api`.
- Dataset và document index trong RAGFlow.

Điều này làm phát sinh sai lệch trạng thái, tăng chi phí embedding, khó xác định nguồn retrieval chính xác và làm phức tạp việc xóa/re-index tài liệu.

## 3. Quyết định kiến trúc đề xuất

### 3.1. Phương án khuyến nghị

Dùng RAGFlow làm hệ thống ingestion và RAG engine chính:

```text
Client
  -> docs-hub-api
       -> JWT authentication
       -> project ACL authorization
       -> document metadata/PostgreSQL
       -> original file/object storage
       -> ingestion job/outbox
            -> RAGFlow upload
            -> RAGFlow parse/chunk/embed/index

Client question
  -> docs-hub-api
       -> JWT authentication
       -> project ACL authorization
       -> resolve allowed RAGFlow dataset/document IDs
       -> RAGFlow retrieval hoặc chat completion
       -> normalize answer và citations
       -> Client
```

Phân chia trách nhiệm:

| Thành phần | Trách nhiệm |
|---|---|
| `docs-hub-api` | JWT, ACL, project/document metadata, lifecycle, audit, API contract cho frontend |
| PostgreSQL | Nguồn dữ liệu chuẩn cho nghiệp vụ và mapping ID local với ID RAGFlow |
| Object storage | Lưu file gốc, phục vụ retry/re-upload |
| RAGFlow | Parse, chunk, embedding, index, retrieval, rerank và tùy chọn generation |
| LLM provider | Model chat/embedding được cấu hình bên trong RAGFlow |

### 3.2. Nguyên tắc PostgreSQL local là source of truth

RAGFlow không thay thế database nghiệp vụ của `docs-hub-api`. Sau khi upload thành công lên RAGFlow, hệ thống vẫn phải quản lý toàn bộ quan hệ project và version trong PostgreSQL local.

Các dữ liệu chỉ PostgreSQL local được phép quyết định:

- Project nào sở hữu document.
- User nào được truy cập project.
- Document hiện có những revision nào.
- Revision thuộc `project_version_id` hay `change_request_id` nào.
- Revision nào đang active, published, archived hoặc deleted.
- Remote dataset/document nào tương ứng với local project/revision.
- Trạng thái đồng bộ giữa local và RAGFlow.
- Audit log, người tạo, thời điểm upload và lịch sử thay đổi.

RAGFlow chỉ là derived store/index có thể tái tạo từ:

```text
PostgreSQL local metadata + file gốc trong ObjectStore
```

Mọi API quản lý project, version và document phải đọc/ghi PostgreSQL trước. Không dùng API list dataset/document của RAGFlow làm nguồn dữ liệu trực tiếp cho frontend.

Quy tắc nhất quán:

1. Tạo document/revision và ingestion job trong PostgreSQL trước.
2. Commit transaction local thành công rồi worker mới upload sang RAGFlow.
3. Sau khi RAGFlow trả remote ID, lưu mapping về đúng revision local.
4. Chỉ đánh dấu revision `ready` khi RAGFlow parse/index hoàn tất.
5. Nếu upload hoặc parse thất bại, revision local và lịch sử version vẫn được giữ để retry hoặc audit.
6. Nếu mất toàn bộ index RAGFlow, hệ thống có thể re-index từ metadata local và file gốc.

### 3.3. Các phương án không khuyến nghị

#### Chạy đồng thời pgvector và RAGFlow trong production

Chỉ nên dùng tạm thời trong giai đoạn shadow testing. Không nên coi cả hai là nguồn production chính vì:

- Một tài liệu có thể `ready` ở hệ thống này nhưng `failed` ở hệ thống kia.
- Kết quả retrieval khác nhau do parser, chunking và embedding khác nhau.
- Xóa hoặc cập nhật tài liệu phải đồng bộ hai nơi.
- Khó giải thích citations và truy vết kết quả.

#### Frontend gọi trực tiếp RAGFlow

Không thực hiện vì sẽ:

- Làm lộ API key.
- Bỏ qua JWT và ACL theo project.
- Làm frontend phụ thuộc trực tiếp vào response contract của RAGFlow.
- Không có audit và rate limit thống nhất tại `docs-hub-api`.

## 4. Phân biệt hai loại API key

Việc triển khai cần phân biệt:

1. **RAGFlow API key**
   - Được tạo trong giao diện RAGFlow tại trang API.
   - `docs-hub-api` sử dụng key này để gọi RAGFlow.
   - Gửi bằng header `Authorization: Bearer <RAGFLOW_API_KEY>`.

2. **Model provider API key**
   - Được cấu hình trong RAGFlow cho LLM, embedding model, reranker hoặc OCR/VLM.
   - RAGFlow sử dụng key này để gọi model provider.
   - Không phải key mà `docs-hub-api` dùng để gọi RAGFlow.

Hai loại key phải được quản lý độc lập. RAGFlow API key không tự cung cấp model cho RAGFlow.

## 5. Công việc phía RAGFlow

### 5.1. Chuẩn bị instance

- Chọn RAGFlow Cloud hoặc self-hosted.
- Xác định `base_url` chính xác cho từng môi trường.
- Chọn và pin phiên bản RAGFlow đối với self-hosted.
- Kiểm tra network từ API/worker tới RAGFlow.
- Chỉ cho phép HTTPS ở staging và production.
- Nếu self-hosted, cấu hình backup và giám sát các storage/index dependency của RAGFlow.

### 5.2. Cấu hình model

Tối thiểu cần:

- Một embedding model phù hợp ngôn ngữ tài liệu.
- Một chat model nếu dùng RAGFlow chat completion.
- Một reranker nếu yêu cầu chất lượng retrieval cao hơn.
- OCR/VLM nếu có PDF scan hoặc tài liệu nhiều hình ảnh.

Cần kiểm thử riêng với tài liệu tiếng Việt. Không nên mặc định model hoạt động tốt chỉ vì hỗ trợ multilingual.

Lưu ý: embedding model của một dataset không nên thay đổi sau khi dataset đã có chunk. Nếu cần đổi model, phải có kế hoạch re-index dataset.

### 5.3. Tạo API key

- Tạo key riêng cho từng môi trường: development, staging và production.
- Không dùng chung key cá nhân giữa các môi trường.
- Ghi nhận owner, ngày tạo và quy trình rotate/revoke.
- Kiểm thử key bằng API liệt kê dataset.

Tài liệu chính thức:

- [Acquire RAGFlow API key](https://github.com/infiniflow/ragflow/blob/main/docs/develop/acquire_ragflow_api_key.md)
- [RAGFlow HTTP API reference](https://github.com/infiniflow/ragflow/blob/main/docs/references/http_api_reference.md)

## 6. Chiến lược dataset và ACL

### 6.1. Mapping khuyến nghị

```text
docs-hub project        1 -> 1 RAGFlow dataset
document revision      1 -> 1 RAGFlow document
```

Mapping này không chuyển quyền sở hữu dữ liệu sang RAGFlow:

| Dữ liệu local | Mapping remote | Mục đích |
|---|---|---|
| `projects.id` | `projects.ragflow_dataset_id` | Xác định index RAGFlow của project |
| `documents.id` | Không bắt buộc có remote ID riêng | Giữ danh tính logic của tài liệu xuyên suốt các revision |
| `document_revisions.id` | `document_revisions.ragflow_document_id` | Mỗi revision được index độc lập và có thể truy vết/xóa riêng |
| `project_versions.id` | Metadata/filter hoặc bảng association local | Chọn đúng tập revision khi query một version |
| `change_requests.id` | Metadata/filter hoặc bảng association local | Cô lập tài liệu đang thay đổi khỏi version đã publish |

`documents` là logical aggregate local; RAGFlow document đại diện cho một revision cụ thể. Vì vậy không được ghi đè remote document của revision cũ trước khi revision mới parse thành công.

Ưu điểm của một dataset cho mỗi project:

- Dataset ID trở thành ranh giới retrieval rõ ràng.
- Backend dễ giới hạn truy vấn theo project đã được authorize.
- Dễ xóa, re-index hoặc đối soát theo project.
- Hạn chế nguy cơ một truy vấn vô tình lấy chunk từ project khác.

Nếu số project cực lớn, cần benchmark và xác nhận giới hạn vận hành của RAGFlow trước khi chuyển sang mô hình dùng chung dataset + metadata filter.

### 6.2. Quy tắc bắt buộc cho retrieval

Backend phải thực hiện theo thứ tự:

1. Xác thực JWT.
2. Kiểm tra user là member của project và có role phù hợp.
3. Đọc `ragflow_dataset_id` từ PostgreSQL bằng project đã được authorize.
4. Tự xây dựng request gửi RAGFlow.
5. Không sử dụng trực tiếp `dataset_ids` do client truyền lên.

Khi request có scope version/change request, backend còn phải:

1. Xác nhận version/change request thuộc đúng project.
2. Truy vấn PostgreSQL để lấy danh sách revision hợp lệ trong scope đó.
3. Map sang `ragflow_document_id`.
4. Gửi `document_ids` hoặc metadata condition đã được backend xây dựng sang RAGFlow.

Như vậy một RAGFlow dataset có thể chứa nhiều revision của cùng project, nhưng query chỉ nhìn thấy tập revision mà PostgreSQL local cho phép.

Quyền `me/team` của RAGFlow không thay thế ACL `project_members` của ứng dụng.

### 6.3. Metadata hỗ trợ phòng thủ nhiều lớp

Nếu API RAGFlow tại phiên bản triển khai hỗ trợ metadata document, nên gửi kèm:

```json
{
  "project_id": "<local-project-id>",
  "document_id": "<local-document-id>",
  "revision_id": "<local-revision-id>",
  "project_version_id": "<optional-version-id>",
  "change_request_id": "<optional-change-request-id>"
}
```

Sau đó dùng metadata condition khi retrieval nếu cần lọc theo version hoặc change request. Dataset boundary vẫn là lớp bảo vệ chính; metadata filter là lớp bổ sung.

## 7. Thay đổi cấu hình ứng dụng

### 7.1. Thêm config struct

Thêm vào `internal/config/config.go`:

```go
type RAGFlowConfig struct {
    Enabled          bool          `mapstructure:"enabled"`
    BaseURL          string        `mapstructure:"base_url"`
    APIKey           string        `mapstructure:"api_key"`
    Timeout          time.Duration `mapstructure:"timeout"`
    UploadTimeout    time.Duration `mapstructure:"upload_timeout"`
    PollInterval     time.Duration `mapstructure:"poll_interval"`
    MaxPollDuration  time.Duration `mapstructure:"max_poll_duration"`
    DefaultChatID    string        `mapstructure:"default_chat_id"`
}
```

Thêm vào config gốc:

```go
RAGFlow RAGFlowConfig `mapstructure:"ragflow"`
```

### 7.2. YAML không chứa secret

Ví dụ:

```yaml
ragflow:
  enabled: true
  base_url: https://ragflow.example.internal
  timeout: 30s
  upload_timeout: 2m
  poll_interval: 3s
  max_poll_duration: 15m
  default_chat_id: ""
```

API key phải được inject bằng environment variable hoặc secret manager:

```text
APP_RAGFLOW_API_KEY=ragflow-xxxxxxxx
```

### 7.3. Validation và safety check

Khi `ragflow.enabled=true`, ứng dụng phải fail-fast nếu:

- `base_url` rỗng.
- `api_key` rỗng.
- Timeout hoặc poll interval không hợp lệ.
- Production dùng HTTP thay vì HTTPS, trừ khi endpoint là internal và có quyết định bảo mật riêng.

Không đưa API key vào error message hoặc log field.

## 8. RAGFlow HTTP client trong Go

### 8.1. Cấu trúc package đề xuất

```text
internal/infrastructure/ai/ragflow/
  client.go
  dataset.go
  document.go
  retrieval.go
  chat.go
  models.go
  errors.go
  health.go
  client_test.go
```

### 8.2. Trách nhiệm của client

- Chuẩn hóa `base_url` và API version.
- Tự động thêm `Authorization: Bearer ...`.
- Đặt `Content-Type` phù hợp cho JSON hoặc multipart.
- Tôn trọng `context.Context` từ caller.
- Giới hạn kích thước response body.
- Decode cả HTTP status và envelope `code/message/data` của RAGFlow.
- Phân loại lỗi retryable và non-retryable.
- Không log API key, file content hoặc toàn bộ prompt chứa dữ liệu nhạy cảm.
- Đóng response body trong mọi nhánh.
- Hỗ trợ streaming nếu chọn chat completion streaming.

### 8.3. Interface đề xuất

Use case không nên phụ thuộc trực tiếp vào concrete HTTP client:

```go
type RAGService interface {
    CreateDataset(ctx context.Context, input CreateDatasetInput) (Dataset, error)
    UploadDocument(ctx context.Context, datasetID string, file DocumentFile) (RemoteDocument, error)
    StartParsing(ctx context.Context, datasetID string, documentIDs []string) error
    GetDocument(ctx context.Context, datasetID, documentID string) (RemoteDocument, error)
    DeleteDocuments(ctx context.Context, datasetID string, documentIDs []string) error
    Retrieve(ctx context.Context, input RetrievalInput) (RetrievalResult, error)
    Chat(ctx context.Context, input ChatInput) (ChatResult, error)
}
```

Nên tách interface nhỏ hơn nếu module ingestion và chat không cần cùng một tập phương thức.

### 8.4. Endpoint tối thiểu

| Nghiệp vụ | Endpoint |
|---|---|
| Tạo dataset | `POST /api/v1/datasets` |
| Liệt kê dataset | `GET /api/v1/datasets` |
| Upload document | `POST /api/v1/datasets/{dataset_id}/documents` |
| Liệt kê/đọc trạng thái document | `GET /api/v1/datasets/{dataset_id}/documents` |
| Bắt đầu parse built-in pipeline | `POST /api/v1/datasets/{dataset_id}/chunks` |
| Xóa document | `DELETE /api/v1/datasets/{dataset_id}/documents` |
| Retrieval | `POST /api/v1/retrieval` |
| Chat completion | `POST /api/v1/openai/{chat_id}/chat/completions` |

Nếu dataset dùng custom ingestion pipeline của RAGFlow, endpoint bắt đầu ingestion là `POST /api/v1/documents/ingest`, không phải endpoint parse built-in.

### 8.5. Error mapping

Tạo error type chứa tối thiểu:

```go
type APIError struct {
    HTTPStatus int
    Code       int
    Message    string
    Retryable  bool
}
```

Quy tắc gợi ý:

- `401`, `403`: non-retryable; cảnh báo cấu hình/quyền.
- `404`: thường non-retryable; kiểm tra mapping remote ID.
- `408`, `429`, `5xx`, network timeout: retryable.
- JSON/envelope không hợp lệ: technical error; không expose raw response cho client.
- RAGFlow trả HTTP 200 nhưng `code != 0`: vẫn phải coi là lỗi.

## 9. Thay đổi database

### 9.1. Mapping remote resource

Migration mới nên thêm:

```sql
ALTER TABLE projects
    ADD COLUMN ragflow_dataset_id VARCHAR(64);

CREATE UNIQUE INDEX uk_projects_ragflow_dataset_id
    ON projects(ragflow_dataset_id)
    WHERE ragflow_dataset_id IS NOT NULL;

ALTER TABLE document_revisions
    ADD COLUMN ragflow_document_id VARCHAR(64),
    ADD COLUMN ragflow_sync_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    ADD COLUMN ragflow_last_error TEXT,
    ADD COLUMN ragflow_synced_at TIMESTAMPTZ;

CREATE UNIQUE INDEX uk_revisions_ragflow_document_id
    ON document_revisions(ragflow_document_id)
    WHERE ragflow_document_id IS NOT NULL;
```

Tùy cách version được mô hình hóa sau này, có thể cần thêm một bảng association rõ ràng thay vì chỉ dựa vào hai foreign key hiện tại:

```sql
CREATE TABLE project_version_document_revisions (
    project_version_id UUID NOT NULL,
    document_revision_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_version_id, document_revision_id)
);
```

Bảng này chỉ cần thiết nếu một revision có thể xuất hiện trong nhiều project version hoặc một project version cần snapshot đầy đủ tập revision. Trước khi thêm phải chốt semantics version của domain; không tạo bảng nếu quan hệ hiện tại đã đáp ứng đúng nghiệp vụ.

Trạng thái đề xuất:

```text
pending -> uploading -> parsing -> ready
                       \-> failed
ready -> deleting -> deleted
```

Nếu vẫn dùng cột `document_revisions.status`, cần xác định rõ cột đó là trạng thái nghiệp vụ tổng hay trạng thái RAGFlow. Không nên có hai state machine chồng chéo mà không có quy tắc ưu tiên.

### 9.2. Có giữ `document_chunks` hay không

Khi RAGFlow trở thành nguồn retrieval chính:

- Dừng ghi chunk mới vào `document_chunks`.
- Chưa xóa bảng ngay trong release đầu để có khả năng rollback.
- Sau giai đoạn ổn định, tạo migration riêng để loại bỏ pgvector/index/chunk nếu không còn consumer.
- Không xóa migration cũ đã được deploy.

### 9.3. Có cần lưu dataset ID trong config không

Không nên dùng một `default_dataset_id` duy nhất nếu dữ liệu được chia theo project. Dataset ID phải nằm trong PostgreSQL để quản lý lifecycle. Config chỉ nên chứa default ID cho thử nghiệm hoặc một knowledge base global thật sự.

### 9.4. Dữ liệu bắt buộc lưu local và remote references

PostgreSQL local vẫn bắt buộc vì RAGFlow không phải hệ thống quản lý domain project/version. RAGFlow chỉ quản lý knowledge base và index phục vụ truy hồi.

Phân quyền sở hữu dữ liệu:

| Loại dữ liệu | Nơi quản lý chính | Ghi chú |
|---|---|---|
| Project, member và ACL | PostgreSQL local | Không giao quyền quyết định cho RAGFlow |
| Logical document | PostgreSQL local | Danh tính tài liệu xuyên suốt nhiều revision |
| Revision, project version và change request | PostgreSQL local | Quyết định revision nào được dùng trong từng scope |
| File gốc và hash | ObjectStore + PostgreSQL local | Nguồn để tải xuống, audit và re-index |
| Parse, chunk, embedding và search index | RAGFlow | Derived data, có thể tái tạo |
| Remote resource references | PostgreSQL local | Cầu nối giữa domain local và RAGFlow |

Remote reference tối thiểu phải lưu:

```text
projects.ragflow_dataset_id
document_revisions.ragflow_document_id
document_revisions.ragflow_sync_status
document_revisions.ragflow_synced_at
document_revisions.ragflow_last_error
```

Reference tùy chọn, chỉ thêm khi có use case:

- `ragflow_chat_id` hoặc `ragflow_assistant_id` nếu mỗi project có assistant riêng.
- `ragflow_session_id` nếu conversation session do RAGFlow quản lý.
- `ragflow_chunk_id` trong bản ghi citation nếu cần audit câu trả lời hoặc mở đúng chunk.

Không cần sao chép toàn bộ chunk và embedding về PostgreSQL nếu RAGFlow là retrieval engine chính. Chỉ tiếp tục lưu local chunks khi có consumer độc lập, yêu cầu audit đầy đủ hoặc rollback đã được xác định rõ.

### 9.5. Quan hệ project, version, revision và RAGFlow

Mô hình quản lý đề xuất:

```text
Local Project A
├── ragflow_dataset_id = dataset_A
├── Local Document X
│   ├── Revision 1 -> ragflow_document_id = remote_X_v1
│   └── Revision 2 -> ragflow_document_id = remote_X_v2
└── Local Document Y
    └── Revision 1 -> ragflow_document_id = remote_Y_v1
```

Nếu Project Version 2 sử dụng `X revision 2` và `Y revision 1`, PostgreSQL phải lưu tập revision này. Khi query Version 2, backend resolve thành:

```json
{
  "dataset_ids": ["dataset_A"],
  "document_ids": ["remote_X_v2", "remote_Y_v1"]
}
```

Không query toàn bộ `dataset_A` khi request bị giới hạn theo version, vì dataset có thể đồng thời chứa revision cũ, revision mới và tài liệu thuộc change request chưa publish.

Nếu một revision chỉ thuộc đúng một project version, foreign key hiện tại có thể đủ. Nếu một revision được tái sử dụng trong nhiều version hoặc mỗi version cần một snapshot đầy đủ, dùng bảng `project_version_document_revisions` để biểu diễn quan hệ nhiều-nhiều.

### 9.6. Quản lý citations/references của câu trả lời

Khi RAGFlow trả citation, backend phải map remote reference về domain local:

```text
RAGFlow dataset_id  -> projects.ragflow_dataset_id -> projects.id
RAGFlow document_id -> document_revisions.ragflow_document_id
                    -> document_revisions.id
                    -> documents.id
```

Trước khi trả citation cho client, backend phải kiểm tra revision vừa map:

- Thuộc project đã được authorize.
- Thuộc version/change request đang được query.
- Chưa bị xóa hoặc archive ngoài scope cho phép.

Response public nên trả local IDs thay vì bắt frontend quản lý RAGFlow IDs:

```json
{
  "document_id": "local-document-id",
  "revision_id": "local-revision-id",
  "title": "Document title",
  "excerpt": "Relevant content",
  "score": 0.82
}
```

Nếu hệ thống lưu lịch sử chat, nên lưu một citation snapshot cùng message, gồm local document/revision ID, remote chunk ID nếu có, excerpt và score tại thời điểm trả lời. Snapshot giúp audit được câu trả lời cũ ngay cả khi tài liệu remote đã re-index hoặc bị xóa. Citation snapshot không thay thế document/revision local và không được dùng để cấp quyền truy cập.

## 10. Thay đổi ingestion flow

### 10.1. Luồng đề xuất

```text
1. Client hoàn tất upload vào docs-hub-api.
2. API commit document revision và ingestion job.
3. Worker claim job bằng FOR UPDATE SKIP LOCKED.
4. Worker kiểm tra/tạo RAGFlow dataset của project.
5. Worker đọc file gốc từ ObjectStore.
6. Worker upload multipart sang RAGFlow.
7. Worker lưu ragflow_document_id ngay sau khi upload thành công.
8. Worker gọi endpoint bắt đầu parse.
9. Worker poll trạng thái document hoặc tạo job kiểm tra trạng thái riêng.
10. Khi RAGFlow hoàn tất, cập nhật revision ready và synced_at.
11. Khi lỗi, lưu error đã sanitize và áp dụng retry policy.
```

Sau bước 7, local database phải giữ đủ mapping để điều hành mọi thao tác tiếp theo. Không yêu cầu người dùng hoặc frontend cung cấp RAGFlow document ID.

Ví dụ trạng thái sau khi upload/parse thành công:

```text
projects.id                       = local project UUID
projects.ragflow_dataset_id       = remote dataset ID
documents.id                      = local logical document UUID
document_revisions.id             = local immutable revision UUID
document_revisions.revision_no    = local version sequence
document_revisions.project_version_id/change_request_id
document_revisions.ragflow_document_id = remote indexed document ID
document_revisions.status         = ready
document_revisions.ragflow_sync_status = ready
```

UI quản lý tài liệu/version luôn gọi `docs-hub-api` và hiển thị dữ liệu từ các bảng local. Trạng thái RAGFlow chỉ được phản ánh thông qua các trường sync status đã được backend chuẩn hóa.

### 10.2. Không giữ transaction database trong lúc gọi RAGFlow

HTTP request tới RAGFlow có thể kéo dài. Không được mở PostgreSQL transaction trong toàn bộ upload/parse/poll. Transaction chỉ dùng cho từng bước cập nhật trạng thái ngắn.

### 10.3. Idempotency

RAGFlow upload có thể thành công nhưng worker timeout trước khi lưu remote ID. Khi retry, nguy cơ tạo document trùng.

Cần ít nhất một trong các chiến lược:

- Đặt tên remote document chứa revision UUID và kiểm tra tồn tại trước khi upload lại.
- Lưu một operation record trước khi gọi remote.
- Sau lỗi không xác định, list document theo tên/metadata/hash để reconcile.
- Tạo periodic reconciliation job.

Tên remote gợi ý:

```text
<revision_uuid>__<sanitized_original_file_name>
```

Không dùng file name đơn thuần làm idempotency key vì nhiều revision có thể trùng tên.

### 10.4. Retry policy

Retry với exponential backoff và jitter cho:

- Network error.
- Timeout.
- HTTP `408`, `429`, `5xx`.
- Trạng thái RAGFlow tạm thời chưa hoàn tất.

Không tự động retry vô hạn cho:

- Invalid API key.
- Dataset không có quyền truy cập.
- File type không được hỗ trợ.
- Payload/config parser không hợp lệ.

Phải sử dụng `max_attempts` và `available_at` hiện có của `ingestion_jobs`, đồng thời bổ sung dead-letter/manual retry workflow nếu cần.

### 10.5. Update và delete

Khi có revision mới:

1. Upload và parse revision mới.
2. Chỉ chuyển revision mới thành active/ready khi parse thành công.
3. Sau đó mới xóa hoặc loại revision cũ khỏi RAGFlow.

Trình tự này tránh khoảng thời gian project không có tài liệu khả dụng.

Khi document local bị xóa:

- Tạo async deletion job/outbox event.
- Xóa document remote.
- Retry nếu RAGFlow tạm lỗi.
- Có reconciliation job xử lý orphan ở cả local và remote.

## 11. Query, retrieval và chat

### 11.1. Hai chế độ tích hợp

#### Chế độ A: RAGFlow retrieval, ứng dụng tự generation

Backend gọi:

```http
POST /api/v1/retrieval
Authorization: Bearer <API_KEY>
Content-Type: application/json

{
  "question": "Câu hỏi của người dùng",
  "dataset_ids": ["authorized-dataset-id"],
  "page": 1,
  "page_size": 10,
  "similarity_threshold": 0.2,
  "vector_similarity_weight": 0.3,
  "keyword": true,
  "highlight": false
}
```

Ưu điểm:

- Kiểm soát prompt, model và output contract.
- Dễ kết hợp business rules riêng.
- Có thể thay retrieval engine trong tương lai.

Nhược điểm:

- Phải tự xây prompt, citations, streaming và token budget.
- Phải vận hành thêm LLM client.

#### Chế độ B: RAGFlow chat completion

Backend gọi:

```http
POST /api/v1/openai/{chat_id}/chat/completions
Authorization: Bearer <API_KEY>
Content-Type: application/json

{
  "model": "model",
  "messages": [
    {"role": "user", "content": "Câu hỏi của người dùng"}
  ],
  "stream": false,
  "extra_body": {
    "reference": true
  }
}
```

Ưu điểm:

- Nhanh có MVP.
- RAGFlow xử lý retrieval, prompt, model và citations.
- API gần OpenAI-compatible.

Nhược điểm:

- Logic phụ thuộc nhiều hơn vào cấu hình assistant trong RAGFlow.
- Cần mapping chat assistant với dataset/project cẩn thận.
- Khó kiểm soát sâu prompt và retrieval hơn chế độ A.

### 11.2. Khuyến nghị theo giai đoạn

- MVP: dùng chat completion với citations để giảm khối lượng triển khai.
- Sau MVP: đánh giá retrieval API nếu cần prompt orchestration, policy hoặc model routing riêng.

### 11.3. API public của `docs-hub-api`

Không expose nguyên endpoint RAGFlow. Tạo contract riêng, ví dụ:

```http
POST /api/v1/projects/{project_id}/chat
```

Request:

```json
{
  "question": "Quy trình phê duyệt tài liệu là gì?",
  "conversation_id": "optional-local-conversation-id",
  "scope": {
    "project_version_id": "optional",
    "change_request_id": "optional"
  }
}
```

Response chuẩn hóa:

```json
{
  "data": {
    "answer": "...",
    "citations": [
      {
        "document_id": "local-document-id",
        "revision_id": "local-revision-id",
        "title": "Document title",
        "content": "Relevant excerpt",
        "score": 0.82
      }
    ],
    "conversation_id": "..."
  }
}
```

Remote document ID phải được map ngược về local document/revision trước khi trả client. Không nên để frontend phụ thuộc vào RAGFlow IDs.

### 11.4. Streaming

Nếu cần streaming:

- Dùng SSE cho public API.
- Proxy stream từ RAGFlow nhưng vẫn xử lý disconnect/cancellation qua context.
- Chỉ emit các event thuộc public contract của ứng dụng.
- Tách answer delta, citation event, usage và final event.
- Không giữ database transaction trong suốt stream.

## 12. Bootstrap và dependency injection

Các thay đổi dự kiến:

- Khởi tạo RAGFlow client trong `internal/bootstrap/infra.go`.
- Đưa client vào `Infra` dưới dạng interface phù hợp.
- Thêm health checker nếu RAGFlow có health endpoint tương thích với phiên bản triển khai.
- Inject client vào ingestion worker.
- Inject retrieval/chat interface vào module chat mới.
- Khi `ragflow.enabled=false`, dùng disabled/no-op implementation có lỗi rõ ràng, không âm thầm trả kết quả rỗng.

Ứng dụng không nhất thiết fail startup khi RAGFlow tạm thời unavailable nếu muốn API metadata vẫn hoạt động. Tuy nhiên phải fail startup nếu config/key bị thiếu trong khi integration được bật.

## 13. Security và compliance

### 13.1. Secret management

- API key chỉ tồn tại ở backend/worker.
- Production lấy key từ secret manager hoặc environment injection.
- Redact `Authorization` trong HTTP logs và traces.
- Không dump toàn bộ request object nếu object chứa key hoặc nội dung tài liệu.
- Có quy trình rotate key không downtime.

### 13.2. Data residency

Trước khi dùng RAGFlow Cloud phải xác nhận:

- File gốc được lưu ở khu vực nào.
- Chunk, embedding, prompt và conversation được lưu bao lâu.
- RAGFlow hoặc model provider có dùng dữ liệu để huấn luyện hay không.
- Cơ chế xóa dữ liệu và backup retention.
- Yêu cầu pháp lý/nội bộ đối với tài liệu doanh nghiệp.

Nếu không đáp ứng, dùng RAGFlow self-hosted và model endpoint private.

### 13.3. Prompt injection và output safety

- Xem nội dung tài liệu là dữ liệu không tin cậy.
- Không cho nội dung retrieved tự động kích hoạt tool hoặc API có side effect.
- Giới hạn độ dài question, history và số chunk.
- Escape/normalize output trước khi render HTML.
- Trả citations để người dùng kiểm chứng.
- Có empty-response policy khi retrieval không đủ bằng chứng.

### 13.4. Tenant isolation test

Phải có test cố gắng:

- User project A gửi project ID của B.
- User project A gửi RAGFlow dataset ID của B trong payload tùy chỉnh.
- User không thuộc project gọi chat/retrieval.
- Citation remote trỏ tới document không thuộc project đã authorize.

Mọi trường hợp phải bị chặn hoặc loại bỏ trước khi trả response.

## 14. Observability

### 14.1. Log

Structured log tối thiểu:

- `request_id`/trace ID.
- Local project/document/revision/job ID.
- Remote dataset/document ID khi không bị policy coi là nhạy cảm.
- Operation: create dataset, upload, parse, poll, retrieve hoặc chat.
- Duration, HTTP status, RAGFlow code và retry attempt.

Không log:

- API key.
- Toàn bộ tài liệu.
- Toàn bộ prompt/answer ở production nếu chưa có chính sách dữ liệu.

### 14.2. Metrics

Đề xuất:

- `ragflow_requests_total{operation,status}`.
- `ragflow_request_duration_seconds{operation}`.
- `ragflow_retries_total{operation}`.
- `ragflow_ingestion_jobs{status}`.
- `ragflow_ingestion_duration_seconds`.
- `ragflow_retrieval_chunks`.
- `ragflow_chat_time_to_first_token_seconds` nếu streaming.

### 14.3. Alert

- Tỷ lệ ingestion failed vượt ngưỡng.
- Queue pending tăng liên tục.
- RAGFlow `401/403` xuất hiện.
- RAGFlow latency hoặc timeout tăng.
- Document ở trạng thái `uploading/parsing` quá lâu.

## 15. Kiểm thử

### 15.1. Unit test

- Authorization header được thêm đúng và được redact khỏi log.
- Decode success envelope.
- HTTP 200 nhưng RAGFlow `code != 0` được trả thành error.
- Phân loại retryable/non-retryable.
- Multipart upload đúng tên field `file`.
- Context cancellation dừng request.
- Mapping remote citation về local document.
- ACL được kiểm tra trước khi gọi mock RAGFlow.

### 15.2. Contract test

Dùng `httptest.Server` giả lập:

- Create/list dataset.
- Upload document.
- Start parse.
- Poll từ running sang done.
- Parse failure.
- Retrieval response.
- Non-stream và stream chat completion.
- Malformed/oversized response.

### 15.3. Integration test

Chạy với RAGFlow test instance:

1. Tạo project/dataset.
2. Upload tài liệu nhỏ có nội dung xác định.
3. Đợi parse hoàn tất.
4. Hỏi câu có đáp án trong tài liệu.
5. Kiểm tra answer và citation thuộc đúng document.
6. Hỏi bằng user không có quyền và xác nhận request bị chặn.
7. Xóa revision và xác nhận remote document được dọn.

Không chạy integration test thật trong unit-test suite mặc định nếu cần hạ tầng nặng hoặc API key.

### 15.4. RAG evaluation

Chuẩn bị bộ câu hỏi/đáp án thực tế, ưu tiên tiếng Việt:

- Exact fact retrieval.
- Câu hỏi cần tổng hợp nhiều đoạn.
- Câu hỏi không có đáp án.
- Tài liệu có nội dung mâu thuẫn theo revision.
- Câu hỏi ngoài quyền truy cập.

Theo dõi:

- Recall@K/retrieval hit rate.
- Citation precision.
- Faithfulness/groundedness.
- Empty-answer correctness.
- Latency và chi phí trên mỗi câu hỏi.

## 16. Kế hoạch rollout

### Giai đoạn 0: quyết định và spike

- Chọn cloud/self-hosted và pin version.
- Chọn model, chunk method và dataset strategy.
- Dùng curl kiểm tra create dataset, upload, parse, retrieval/chat.
- Đo chất lượng với một tập tài liệu đại diện.

Điều kiện hoàn tất:

- API key và network hoạt động.
- Tài liệu parse thành công.
- Retrieval/chat trả đúng citation cơ bản.

### Giai đoạn 1: nền tảng client

- Thêm config và validation.
- Implement HTTP client và error mapping.
- Thêm health/metrics/log redaction.
- Unit và contract tests.

### Giai đoạn 2: ingestion integration

- Thêm migration remote IDs/status.
- Mapping project -> dataset.
- Thay worker path bằng RAGFlow khi feature flag bật.
- Retry, polling và reconciliation.
- Giữ pipeline pgvector cũ làm rollback path nhưng không dùng đồng thời cho response production.

### Giai đoạn 3: query API

- Thêm chat/retrieval vertical slice.
- Enforce ACL.
- Normalize citations.
- Thêm streaming nếu cần.
- Audit và rate limit.

### Giai đoạn 4: shadow/canary

- Chạy RAGFlow trên một số project thử nghiệm.
- So sánh retrieval hiện tại và RAGFlow offline.
- Theo dõi lỗi, latency, chất lượng và chi phí.
- Không trộn kết quả từ hai engine trong response cho user.

### Giai đoạn 5: cutover và cleanup

- Chuyển toàn bộ project theo batch.
- Reconcile đủ document/revision.
- Tắt ghi chunk/embedding local.
- Sau thời gian rollback đã thống nhất, loại bỏ LocalAI embedding và `document_chunks` nếu không còn consumer.
- Cập nhật ADR-0006 hoặc tạo ADR mới ghi nhận RAGFlow là RAG engine chính.

## 17. Feature flags và rollback

Feature flag đề xuất:

```yaml
ragflow:
  enabled: true
  ingestion_enabled: true
  query_enabled: true
```

Có thể thêm engine selection trong giai đoạn chuyển đổi:

```yaml
rag:
  ingestion_engine: ragflow
  query_engine: ragflow
```

Rollback chỉ an toàn khi:

- File gốc vẫn còn trong ObjectStore.
- Local metadata và revision không phụ thuộc độc quyền vào remote response.
- Chưa xóa bảng/chunk local trong giai đoạn canary.
- Có danh sách project/revision đã sync để re-index lại nếu cần.

Không nên tự động fallback sang pgvector trong cùng một request mà không ghi nhận, vì kết quả và ACL behavior có thể khác nhau.

## 18. Danh sách file dự kiến thay đổi

```text
internal/config/config.go
internal/config/load.go
internal/config/load_test.go
configs/config.local.yaml
configs/config.dev.yaml
configs/config.staging.yaml
configs/config.production.yaml

internal/infrastructure/ai/ragflow/client.go
internal/infrastructure/ai/ragflow/models.go
internal/infrastructure/ai/ragflow/dataset.go
internal/infrastructure/ai/ragflow/document.go
internal/infrastructure/ai/ragflow/retrieval.go
internal/infrastructure/ai/ragflow/chat.go
internal/infrastructure/ai/ragflow/errors.go
internal/infrastructure/ai/ragflow/health.go
internal/infrastructure/ai/ragflow/*_test.go

internal/bootstrap/infra.go
internal/bootstrap/modules.go

internal/module/ingestion/processor.go
internal/module/ingestion/processor_test.go

internal/module/chat/domain/*
internal/module/chat/usecase/*
internal/module/chat/delivery/http/*
internal/module/chat/module.go

migrations/000005_add_ragflow_mapping.up.sql
migrations/000005_add_ragflow_mapping.down.sql

docs/architecture/ADR-0007-ragflow-integration.md
docs/api/openapi/chat/*
docs/swagger/*
```

Tên migration thực tế phải điều chỉnh theo migration mới nhất tại thời điểm implement.

## 19. Đối chiếu endpoint RAGFlow theo chức năng cần chỉnh sửa

Phần này sử dụng danh sách endpoint RAGFlow được cung cấp cho dự án và map từng nhóm chức năng vào thay đổi cần thực hiện trong `docs-hub-api`.

Danh sách này phải được xác nhận lại với đúng phiên bản RAGFlow được pin trước khi code. Không suy ra endpoint chỉ từ tên chức năng: method, payload, response envelope, trạng thái parse và deprecated alias có thể thay đổi giữa các phiên bản.

Quy ước mức độ:

- **Bắt buộc**: cần cho luồng upload, quản lý project/version và hỏi đáp cơ bản.
- **Có điều kiện**: chỉ implement khi chọn chế độ hoặc tính năng tương ứng.
- **Ngoài scope MVP**: chưa implement; cần ADR/scope riêng nếu bổ sung sau.
- **Không dùng làm source of truth**: có thể gọi phục vụ kỹ thuật nhưng không thay thế PostgreSQL/ObjectStore local.

### 19.1. System health

| Endpoint | Mức độ | Chức năng trong hệ thống | Phần cần chỉnh sửa |
|---|---|---|---|
| `GET /api/v1/system/healthz` | Bắt buộc | Kiểm tra RAGFlow có reachable/healthy | Thêm `ragflow.HealthChecker`; inject vào `Infra.Checkers`; expose trạng thái qua admin health nhưng không trả API key/base URL nhạy cảm |

Client method:

```go
Health(ctx context.Context) error
```

File dự kiến:

```text
internal/infrastructure/ai/ragflow/health.go
internal/infrastructure/ai/ragflow/health_test.go
internal/bootstrap/infra.go
internal/module/health/health.go
```

Ứng dụng phải phân biệt:

- Config sai hoặc thiếu API key khi integration bật: fail startup.
- RAGFlow tạm unavailable lúc startup: có thể cho API metadata khởi động ở trạng thái degraded; ingestion/chat trả dependency unavailable.

### 19.2. Dataset management

Mỗi project local được đề xuất map với một RAGFlow dataset.

| Endpoint | Mức độ | Cách sử dụng | Dữ liệu local liên quan |
|---|---|---|---|
| `POST /api/v1/datasets` | Bắt buộc | Tạo dataset khi project chưa có remote dataset | `projects.id`, `projects.ragflow_dataset_id`, sync status |
| `GET /api/v1/datasets?...` | Bắt buộc | Reconcile, kiểm tra dataset tồn tại, phục hồi khi create timeout | Không dùng kết quả làm danh sách project cho UI |
| `PUT /api/v1/datasets/{dataset_id}` | Có điều kiện | Đồng bộ tên/mô tả hoặc parser config khi policy cho phép | Project local vẫn là nguồn tên/mô tả chuẩn |
| `DELETE /api/v1/datasets` | Bắt buộc cho lifecycle | Xóa remote dataset sau khi project local đã vào quy trình xóa | Thực hiện async/outbox, không xóa nhầm dataset đang được mapping |
| `GET /api/v1/datasets/{dataset_id}/knowledge_graph` | Ngoài scope MVP | Đọc knowledge graph | Chỉ thêm khi UI/use case graph được duyệt |
| `DELETE /api/v1/datasets/{dataset_id}/knowledge_graph` | Ngoài scope MVP | Xóa graph artifacts | Không gắn với xóa project thông thường nếu chưa bật GraphRAG |
| `POST /api/v1/datasets/{dataset_id}/run_graphrag` | Ngoài scope MVP | Xây GraphRAG | Cần job/status/timeout riêng |
| `GET /api/v1/datasets/{dataset_id}/trace_graphrag` | Ngoài scope MVP | Theo dõi GraphRAG | Cần poller và normalized status nếu triển khai |
| `POST /api/v1/datasets/{dataset_id}/run_raptor` | Ngoài scope MVP | Xây RAPTOR | Cần benchmark chất lượng/chi phí trước |
| `GET /api/v1/datasets/{dataset_id}/trace_raptor` | Ngoài scope MVP | Theo dõi RAPTOR | Cần poller và metrics riêng nếu triển khai |

Client methods tối thiểu:

```go
CreateDataset(ctx context.Context, input CreateDatasetInput) (Dataset, error)
ListDatasets(ctx context.Context, filter DatasetFilter) ([]Dataset, error)
UpdateDataset(ctx context.Context, datasetID string, input UpdateDatasetInput) error
DeleteDatasets(ctx context.Context, datasetIDs []string) error
```

Thay đổi domain/repository:

- Thêm `ragflow_dataset_id` và trạng thái provisioning vào project hoặc bảng integration mapping.
- Create dataset phải idempotent. Nếu remote create thành công nhưng local update timeout, reconcile bằng tên kỹ thuật chứa project UUID hoặc metadata phù hợp.
- Không chấp nhận `dataset_id` tùy ý từ client public.
- Delete dataset phải chạy sau kiểm tra mapping và theo outbox/retry.

Tên dataset kỹ thuật gợi ý:

```text
project_<project_uuid>
```

Tên hiển thị có thể chứa project name, nhưng UUID phải giữ được khả năng reconcile khi tên project thay đổi.

### 19.3. File/document management trong dataset

Đây là nhóm endpoint chính của ingestion.

| Endpoint | Mức độ | Cách sử dụng | Phần cần chỉnh sửa |
|---|---|---|---|
| `POST /api/v1/datasets/{dataset_id}/documents` | Bắt buộc | Worker upload file gốc vào dataset project | Thay bước parse/embed local trong ingestion processor bằng multipart upload RAGFlow |
| `GET /api/v1/datasets/{dataset_id}/documents?...` | Bắt buộc | Poll parse status, tìm document để reconcile | Map remote document về `document_revisions.ragflow_document_id` |
| `PUT /api/v1/datasets/{dataset_id}/documents/{document_id}` | Có điều kiện | Update tên, parser config hoặc metadata remote | Chỉ expose qua use case local; không cho client gọi passthrough |
| `GET /api/v1/datasets/{dataset_id}/documents/{document_id}` | Có điều kiện | Download/đối soát file remote | Public download vẫn ưu tiên ObjectStore local |
| `DELETE /api/v1/datasets/{dataset_id}/documents` | Bắt buộc | Xóa revision remote khi archive/delete hoặc cleanup orphan | Async deletion job, retry và audit |
| `POST /api/v1/datasets/{dataset_id}/chunks` | Bắt buộc nếu dùng built-in chunking | Bắt đầu parse các document vừa upload | Lưu trạng thái `parsing`, poll đến done/fail |
| `POST /api/v1/documents/ingest` | Có điều kiện | Bắt đầu/cancel custom ingestion pipeline | Dùng thay endpoint `/chunks`, không gọi cả hai |
| `DELETE /api/v1/datasets/{dataset_id}/chunks` | Có điều kiện | Dừng parsing built-in | Dùng cho cancel job/manual recovery |

Client methods:

```go
UploadDocuments(ctx context.Context, datasetID string, files []DocumentFile) ([]RemoteDocument, error)
ListDocuments(ctx context.Context, datasetID string, filter DocumentFilter) (DocumentPage, error)
GetDocument(ctx context.Context, datasetID, documentID string) (RemoteDocument, error)
UpdateDocument(ctx context.Context, datasetID, documentID string, input UpdateDocumentInput) error
DeleteDocuments(ctx context.Context, datasetID string, documentIDs []string) error
StartParsing(ctx context.Context, datasetID string, documentIDs []string) error
StopParsing(ctx context.Context, datasetID string, documentIDs []string) error
IngestDocuments(ctx context.Context, input IngestDocumentsInput) error
```

Quy tắc implement:

1. `document_revisions` local được tạo trước remote upload.
2. File gốc vẫn lưu local ObjectStore; remote file không phải bản backup duy nhất.
3. Worker resolve `dataset_id` từ project local.
4. Upload thành công thì lưu `ragflow_document_id` ngay.
5. Chọn duy nhất một mode:
   - Built-in chunking: `POST .../documents` rồi `POST .../chunks`.
   - Custom pipeline: upload/link phù hợp rồi `POST /api/v1/documents/ingest`.
6. Poll bằng list/get document, normalize trạng thái RAGFlow về local sync state.
7. Chỉ đánh dấu revision `ready` khi remote parse/index hoàn tất.
8. Khi revision mới ready, mới archive/xóa remote revision cũ theo policy.

Public API không trả thẳng remote document object. Response document vẫn dùng local `document_id`, `revision_id`, version và trạng thái chuẩn hóa.

### 19.4. Chunk management, metadata và retrieval

| Endpoint | Mức độ | Cách sử dụng | Phần cần chỉnh sửa |
|---|---|---|---|
| `POST /api/v1/retrieval` | Bắt buộc nếu backend tự kiểm soát retrieval | Truy hồi chunk theo dataset/document đã authorize | Tạo retrieval use case, ACL, version resolver và citation mapper |
| `GET /api/v1/datasets/{dataset_id}/documents/{document_id}/chunks?...` | Có điều kiện | Debug/admin xem chunk hoặc kiểm thử ingestion | Chỉ admin/internal API; không dùng để list document nghiệp vụ |
| `GET .../chunks/{chunk_id}` | Có điều kiện | Mở chi tiết citation hoặc debug | Luôn map/check project + revision trước khi gọi |
| `POST .../documents/{document_id}/chunks` | Ngoài scope MVP | Thêm chunk thủ công | Cần audit và quy tắc re-index riêng |
| `PATCH .../chunks/{chunk_id}` | Ngoài scope MVP | Sửa chunk thủ công | Không cho sửa nếu canonical source vẫn là file local mà không có reconciliation |
| `PATCH .../documents/{document_id}/chunks` | Ngoài scope MVP | Bật/tắt availability của chunk | Cần lưu override local nếu muốn tồn tại qua re-index |
| `DELETE .../documents/{document_id}/chunks` | Ngoài scope MVP | Xóa chunk thủ công | Dễ làm remote lệch file/revision local |
| `GET /api/v1/datasets/{dataset_id}/metadata/summary` | Có điều kiện | Khám phá metadata để xây filter/admin UI | Không dùng làm schema domain chuẩn |
| `POST /api/v1/datasets/{dataset_id}/metadata/update` | Có điều kiện | Đồng bộ metadata project/version/revision | Chỉ backend xây payload từ DB local |

Retrieval flow bắt buộc:

```text
request project/version/change-request scope
  -> verify JWT
  -> verify project membership/role
  -> validate scope belongs to project
  -> PostgreSQL resolve allowed local revisions
  -> map allowed revisions to ragflow_document_id
  -> call POST /api/v1/retrieval with dataset_ids + document_ids
  -> discard any chunk outside the allowed mapping
  -> map citations to local IDs
  -> return normalized response
```

Client methods:

```go
Retrieve(ctx context.Context, input RetrievalInput) (RetrievalResult, error)
ListChunks(ctx context.Context, datasetID, documentID string, filter ChunkFilter) (ChunkPage, error)
GetChunk(ctx context.Context, datasetID, documentID, chunkID string) (Chunk, error)
GetMetadataSummary(ctx context.Context, datasetID string) (MetadataSummary, error)
UpdateMetadata(ctx context.Context, datasetID string, input MetadataUpdateInput) error
```

Không cần implement các method mutation chunk trong MVP. Nếu sau này cho phép sửa chunk, phải quyết định thay đổi đó có được ghi về canonical document local hay sẽ bị mất khi re-index.

### 19.5. OpenAI-compatible completion

| Endpoint | Mức độ | Cách sử dụng | Phần cần chỉnh sửa |
|---|---|---|---|
| `POST /api/v1/openai/{chat_id}/chat/completions` | Bắt buộc nếu chọn RAGFlow chat cho MVP | Sinh answer và citation bằng chat assistant | Tạo chat client, SSE/non-stream decoder, citation mapper và public chat API |
| `POST /api/v1/agents_openai/{agent_id}/chat/completions` | Ngoài scope MVP | Chạy RAGFlow Agent qua OpenAI-compatible API | Chỉ thêm khi có agent workflow/tool policy |

Client method đề xuất:

```go
CreateChatCompletion(ctx context.Context, chatID string, input ChatCompletionInput) (ChatCompletion, error)
StreamChatCompletion(ctx context.Context, chatID string, input ChatCompletionInput) (ChatStream, error)
```

Các điểm cần implement:

- Lấy `chat_id` từ mapping/config backend, không tin `chat_id` do client truyền.
- Với `stream=true`, parse SSE đến `[DONE]`, propagate context cancellation và đóng body.
- Yêu cầu references trong request khi cần citations.
- Map remote `dataset_id`, `document_id`, chunk reference về local IDs.
- Lọc bỏ citation không thuộc project/version đã authorize.
- Chuẩn hóa usage/token và error về response envelope của `docs-hub-api`.
- Không expose raw RAGFlow model/config cho frontend nếu không nằm trong public contract.

Nếu chat assistant chỉ gắn dataset mà không giới hạn được đúng revision/version, không dùng endpoint này cho scope version nghiêm ngặt trừ khi metadata filter đã được cấu hình và kiểm thử. Khi cần giới hạn tuyệt đối theo `document_ids`, ưu tiên retrieval API rồi tự generation.

### 19.6. Chat assistant management

Nhóm này chỉ bắt buộc nếu chọn RAGFlow quản lý assistant cho chat completion.

| Endpoint | Mức độ | Cách sử dụng | Local mapping |
|---|---|---|---|
| `POST /api/v1/chats` | Có điều kiện | Tạo assistant gắn dataset project hoặc assistant dùng chung | `projects.ragflow_chat_id` hoặc integration config |
| `GET /api/v1/chats/{chat_id}` | Có điều kiện | Kiểm tra/reconcile assistant | Không dùng làm project config source of truth |
| `GET /api/v1/chats?...` | Có điều kiện | Reconcile/list admin | Không expose trực tiếp cho user thường |
| `PUT /api/v1/chats/{chat_id}` | Có điều kiện | Replace cấu hình assistant | Chỉ gọi qua admin/config workflow |
| `PATCH /api/v1/chats/{chat_id}` | Có điều kiện | Partial update model/prompt/datasets | Cần audit thay đổi prompt/model |
| `DELETE /api/v1/chats/{chat_id}` | Có điều kiện | Xóa một assistant | Async và kiểm tra mapping |
| `DELETE /api/v1/chats` | Có điều kiện | Batch cleanup assistants | Chỉ internal/admin job |

Cần chốt một trong hai mô hình:

1. **Một assistant/project**: cô lập config tốt nhưng nhiều remote resource; lưu `projects.ragflow_chat_id`.
2. **Một assistant dùng chung**: ít resource hơn nhưng phải chứng minh request có thể filter đúng dataset/version trên từng call.

Không cho phép người dùng thường sửa system prompt, model hoặc dataset binding trực tiếp qua passthrough API.

### 19.7. Session management và conversation

| Endpoint | Mức độ | Cách sử dụng | Quyết định cần chốt |
|---|---|---|---|
| `POST /api/v1/chats/{chat_id}/sessions` | Có điều kiện | Tạo remote chat session | Dùng khi RAGFlow giữ conversation state |
| `GET /api/v1/chats/{chat_id}/sessions/{session_id}` | Có điều kiện | Đọc/reconcile session | Phải map session với local user/project |
| `GET /api/v1/chats/{chat_id}/sessions?...` | Có điều kiện | List session admin/reconcile | Không dùng làm nguồn conversation trực tiếp nếu local lưu history |
| `PATCH /api/v1/chats/{chat_id}/sessions/{session_id}` | Có điều kiện | Đổi tên/config session | Chỉ backend gọi sau authorization |
| `DELETE /api/v1/chats/{chat_id}/sessions` | Có điều kiện | Xóa session remote | Đồng bộ theo retention/delete local |
| `DELETE .../sessions/{session_id}/messages/{msg_id}` | Có điều kiện | Xóa message | Cần local audit và retention semantics |
| `PUT .../sessions/{session_id}/messages/{msg_id}/feedback` | Có điều kiện | Gửi feedback | Nên lưu feedback local trước hoặc cùng outbox |
| `POST /api/v1/chat/completions` | Có điều kiện | Converse theo chat/session API native | Chọn endpoint này hoặc OpenAI-compatible endpoint, không tạo hai public contract song song |

Khuyến nghị cho hệ thống cần audit/version:

- PostgreSQL local quản lý `conversations`, `messages`, actor, project/version scope và citation snapshot.
- RAGFlow session ID chỉ là optional remote reference.
- Conversation local không được mất khi remote session bị xóa/recreate.
- Mỗi lần hỏi phải authorize lại project/scope; không coi session ID là bằng chứng quyền truy cập.

Schema tùy chọn:

```text
conversations.id
conversations.project_id
conversations.project_version_id/change_request_id
conversations.ragflow_chat_id
conversations.ragflow_session_id
messages.id
messages.conversation_id
messages.ragflow_message_id
message_citations.message_id
message_citations.document_revision_id
message_citations.ragflow_chunk_id
message_citations.excerpt_snapshot
message_citations.score
```

### 19.8. Agent, audio và tiện ích chat

Các endpoint sau là ngoài scope tích hợp RAG cơ bản:

| Nhóm/endpoint | Mức độ | Ghi chú |
|---|---|---|
| `POST /api/v1/agents/chat/completions` | Ngoài scope MVP | Cần tool authorization, sandbox và side-effect policy |
| `GET/DELETE /api/v1/agents/{agent_id}/sessions...` | Ngoài scope MVP | Chỉ triển khai cùng agent feature |
| `GET/POST/PUT/DELETE /api/v1/agents...` | Ngoài scope MVP | Cần module quản trị agent riêng |
| `POST /api/v1/chat/audio/speech` | Ngoài scope MVP | Text-to-speech; cần media streaming/storage policy |
| `POST /api/v1/chat/audio/transcription` | Ngoài scope MVP | Speech-to-text; cần upload limits và privacy review |
| `POST /api/v1/chat/mindmap` | Ngoài scope MVP | Tính năng UI riêng |
| `POST /api/v1/chat/recommandation` | Ngoài scope MVP | Endpoint dùng spelling `recommandation`; bọc trong client để không lan tên API này vào domain |

Không thêm các endpoint này vào interface lõi `RAGService`. Nếu cần, tạo capability interfaces riêng để tránh client lớn và module phụ thuộc ngoài nhu cầu.

### 19.9. Memory management

Toàn bộ nhóm Memory là ngoài scope MVP:

```text
POST   /api/v1/memories
PUT    /api/v1/memories/{memory_id}
GET    /api/v1/memories
GET    /api/v1/memories/{memory_id}/config
DELETE /api/v1/memories/{memory_id}
GET    /api/v1/memories/{memory_id}
POST   /api/v1/messages
DELETE /api/v1/messages/{memory_id}:{message_id}
PUT    /api/v1/messages/{memory_id}:{message_id}
GET    /api/v1/messages/search
GET    /api/v1/messages
GET    /api/v1/messages/{memory_id}:{message_id}/content
```

Chỉ triển khai khi có requirement long-term memory rõ ràng. Cần xác định:

- Memory thuộc user, project, agent hay organization.
- Retention, consent và quyền xóa dữ liệu.
- Có cho phép memory từ project A ảnh hưởng project B không.
- PostgreSQL local lưu ownership và `ragflow_memory_id` như thế nào.
- Message nào được phép đưa vào memory.

Không dùng RAGFlow memory thay thế conversation/audit log local.

### 19.10. File management cấp RAGFlow

RAGFlow có file system riêng ngoài document-in-dataset. Đối với kiến trúc hiện tại, ObjectStore local vẫn là nơi lưu file gốc.

| Endpoint | Mức độ | Cách sử dụng |
|---|---|---|
| `POST /api/v1/files` | Ngoài scope MVP | Upload/tạo file hoặc folder trong RAGFlow file system |
| `GET /api/v1/files?...` | Ngoài scope MVP | Duyệt RAGFlow file system |
| `GET /api/v1/files/{file_id}` | Ngoài scope MVP | Download remote file |
| `GET .../{file_id}/parent`, `GET .../{file_id}/ancestors` | Ngoài scope MVP | Folder navigation |
| `DELETE /api/v1/files` | Ngoài scope MVP | Remote file cleanup |
| `POST /api/v1/files/move` | Ngoài scope MVP | Move/rename remote file |
| `POST /api/v1/files/link-to-datasets` | Có điều kiện | Hữu ích nếu chọn upload một lần vào RAGFlow file system rồi link nhiều dataset |
| `POST /api/v1/documents/upload` | Có điều kiện | Generic upload endpoint; không dùng đồng thời với dataset document upload nếu không có lý do rõ ràng |
| `GET /api/v1/agents/attachments/{attachment_id}/download` | Ngoài scope MVP | Chỉ dùng với agent attachment |

MVP nên dùng trực tiếp:

```text
POST /api/v1/datasets/{dataset_id}/documents
```

vì mapping project dataset đã rõ ràng. Chỉ chuyển sang RAGFlow file system + `link-to-datasets` nếu có requirement cùng một physical file tham gia nhiều dataset và đã chốt lifecycle/reference counting.

### 19.11. Workspace commits và versioning của RAGFlow

Các endpoint:

```text
POST /api/v1/workspaces/{workspace_id}/commits
GET  /api/v1/workspaces/{workspace_id}/commits
GET  /api/v1/workspaces/{workspace_id}/commits/{commit_id}
GET  /api/v1/workspaces/{workspace_id}/commits/{commit_id}/files
GET  /api/v1/workspaces/{workspace_id}/commits/diff
GET  /api/v1/workspaces/{workspace_id}/changes
GET  /api/v1/workspaces/{workspace_id}/commits/{commit_id}/tree
GET  /api/v1/workspaces/{workspace_id}/commits/{commit_id}/files/{file_id}/content
GET  /api/v1/workspace-files/{file_id}/versions
```

Mức độ: **Ngoài scope MVP và không dùng làm source of truth cho project version**.

Lý do:

- Repository đã có `project_versions`, `change_requests`, `documents` và `document_revisions` với domain semantics riêng.
- RAGFlow workspace commit không tự biết ACL, release lifecycle và change request của ứng dụng.
- Nếu dùng song song làm version manager sẽ xuất hiện hai hệ thống lịch sử khó reconcile.

Chỉ dùng nhóm endpoint này sau một ADR riêng cho use case duyệt/diff nội dung ngay trong RAGFlow. Dù triển khai, PostgreSQL local vẫn phải lưu mapping:

```text
local project/version/revision <-> ragflow workspace/commit/file version
```

và vẫn là nguồn dữ liệu quyết định version nào published/archived.

### 19.12. Search app management

| Endpoint | Mức độ | Ghi chú |
|---|---|---|
| `POST /api/v1/searches` | Ngoài scope MVP | Tạo RAGFlow Search App |
| `GET /api/v1/searches?...` | Ngoài scope MVP | List/reconcile search app |
| `GET /api/v1/searches/{search_id}` | Ngoài scope MVP | Đọc cấu hình search app |
| `PUT /api/v1/searches/{search_id}` | Ngoài scope MVP | Update app; cần audit config |
| `DELETE /api/v1/searches/{search_id}` | Ngoài scope MVP | Cleanup remote resource |
| `POST /api/v1/searches/{search_id}/completions` | Ngoài scope MVP | Chỉ dùng khi chọn Search App thay retrieval/chat design hiện tại |

Không implement Search App đồng thời với retrieval API và chat assistant trong MVP. Cần chọn một query path chính để tránh ba contract, ba cấu hình và ba kiểu citation khác nhau.

### 19.13. Phạm vi endpoint đề xuất cho MVP

Endpoint bắt buộc hoặc gần-bắt-buộc nên implement trước:

```text
GET    /api/v1/system/healthz

POST   /api/v1/datasets
GET    /api/v1/datasets
PUT    /api/v1/datasets/{dataset_id}
DELETE /api/v1/datasets

POST   /api/v1/datasets/{dataset_id}/documents
GET    /api/v1/datasets/{dataset_id}/documents
DELETE /api/v1/datasets/{dataset_id}/documents
POST   /api/v1/datasets/{dataset_id}/chunks
DELETE /api/v1/datasets/{dataset_id}/chunks

POST   /api/v1/retrieval
```

Nếu chọn RAGFlow chat completion cho MVP, bổ sung:

```text
POST   /api/v1/chats
GET    /api/v1/chats/{chat_id}
PATCH  /api/v1/chats/{chat_id}
DELETE /api/v1/chats/{chat_id}
POST   /api/v1/openai/{chat_id}/chat/completions
```

Nếu dùng custom ingestion pipeline, thay:

```text
POST /api/v1/datasets/{dataset_id}/chunks
```

bằng:

```text
POST /api/v1/documents/ingest
```

Không implement Agent, Memory, audio, mind map, workspace commits, GraphRAG, RAPTOR và Search App trong cùng milestone MVP.

### 19.14. File/module mapping tổng hợp

| Chức năng | File/module cần tạo hoặc sửa |
|---|---|
| Auth, base URL, HTTP transport | `internal/infrastructure/ai/ragflow/client.go` |
| Dataset CRUD/reconcile | `ragflow/dataset.go`, project use case/repository, bootstrap |
| Document upload/status/delete | `ragflow/document.go`, ingestion processor/repository |
| Parse/ingest/cancel | `ragflow/document.go`, ingestion job state machine |
| Retrieval | `ragflow/retrieval.go`, module chat/search use case |
| Chat completion/SSE | `ragflow/chat.go`, chat HTTP handler |
| Assistant CRUD | `ragflow/assistant.go`, project/integration config service |
| Session/history | `ragflow/session.go`, local conversation/message repositories |
| Citation mapping | chat/retrieval use case + document revision repository |
| Health/metrics | `ragflow/health.go`, bootstrap infra, telemetry |
| Remote ID/status persistence | migration + project/document repositories |
| OpenAPI public contract | `docs/api/openapi/chat/*`, generated Swagger artifacts |

Mỗi file client chỉ chứa DTO và transport-specific logic của nhóm endpoint tương ứng. Domain/usecase không nhận trực tiếp RAGFlow response structs.

## 20. Work breakdown và tiêu chí hoàn tất

### RF-01: Chốt kiến trúc

- Chọn RAGFlow deployment và version.
- Chọn một dataset/project hay shared dataset.
- Chọn retrieval API hay chat completion cho MVP.
- Chọn built-in chunk method hay custom ingestion pipeline.

Hoàn tất khi có ADR được approve.

### RF-02: Cấu hình và secret

- Thêm config struct/default/validation.
- Inject API key bằng environment/secret manager.
- Kiểm tra không log secret.

Hoàn tất khi app fail-fast với config sai và khởi động được với config đúng.

### RF-03: HTTP client

- Implement auth, JSON, multipart, timeout, errors và retries.
- Có unit/contract tests.

Hoàn tất khi các endpoint tối thiểu hoạt động với test server và RAGFlow test instance.

### RF-04: Database mapping

- Thêm remote IDs, sync status và indexes.
- Repository hỗ trợ cập nhật trạng thái an toàn.
- Các API project/document/version tiếp tục lấy PostgreSQL local làm nguồn dữ liệu chuẩn.
- Có truy vấn map version/change request -> local revisions -> RAGFlow document IDs.

Hoàn tất khi migration up/down chạy được, mapping có uniqueness phù hợp và có thể tái tạo toàn bộ index RAGFlow từ local metadata + ObjectStore.

### RF-05: Dataset lifecycle

- Tạo hoặc gắn dataset cho project.
- Xử lý retry/idempotency khi create dataset.

Hoàn tất khi một project luôn resolve được tối đa một active dataset.

### RF-06: Document ingestion

- Upload, parse, poll, retry và failure handling.
- Update/delete/re-index lifecycle.

Hoàn tất khi revision chỉ `ready` sau khi RAGFlow parse xong và job retry không tạo duplicate ngoài kiểm soát.

### RF-07: Chat/retrieval API

- Public request/response contract.
- ACL trước remote call.
- Citation mapping.
- Streaming nếu nằm trong scope.

Hoàn tất khi user chỉ nhận dữ liệu từ project có quyền và citations map được về local document.

### RF-08: Observability và operations

- Metrics, logs, alerts và reconciliation job.
- Runbook rotate key, retry job và xử lý orphan.

Hoàn tất khi có thể phát hiện và xử lý ingestion bị kẹt hoặc remote/local lệch trạng thái.

### RF-09: Quality và security verification

- Tenant isolation tests.
- RAG evaluation dataset.
- Load/latency test.
- Data residency review.

Hoàn tất khi đạt ngưỡng chất lượng, bảo mật và hiệu năng đã thống nhất.

## 21. Các câu hỏi cần chốt trước khi implement

1. Dùng RAGFlow Cloud hay self-hosted?
2. Phiên bản RAGFlow nào sẽ được pin?
3. Tài liệu có được phép rời khỏi hạ tầng nội bộ không?
4. Một project có tương ứng với một dataset không?
5. Có cần phân scope theo `project_version_id` và `change_request_id` khi hỏi không?
6. MVP dùng retrieval API hay RAGFlow chat completion?
7. Có yêu cầu streaming không?
8. Conversation history lưu ở PostgreSQL hay RAGFlow session?
9. Model chat, embedding, reranker và OCR/VLM nào được dùng?
10. Built-in chunking hay custom ingestion pipeline?
11. SLA ingestion và query là bao nhiêu?
12. Chính sách xóa, retention và backup dữ liệu remote là gì?
13. Có cần giữ pgvector làm fallback trong bao lâu?
14. Ngưỡng RAG evaluation nào được coi là đạt?

## 22. Kết luận

Tích hợp RAGFlow không nên được xem là việc chỉ thêm API key vào LocalAI client hiện tại. Đây là thay đổi ranh giới trách nhiệm của kiến trúc RAG.

Hướng triển khai an toàn nhất là:

1. Giữ PostgreSQL local và object storage làm nguồn dữ liệu chuẩn cho project, version, document, revision và file gốc.
2. Dùng RAGFlow làm ingestion và RAG engine chính.
3. Mapping project/revision local với dataset/document remote.
4. Bắt buộc authorize project trước mọi request tới RAGFlow.
5. Tích hợp bất đồng bộ, idempotent, retryable và có reconciliation.
6. Chuyển đổi theo feature flag/canary trước khi loại bỏ pipeline pgvector hiện tại.

API key chỉ giải quyết authentication giữa backend và RAGFlow. Để tích hợp production hoàn chỉnh vẫn cần xử lý lifecycle dữ liệu, ACL, trạng thái bất đồng bộ, error mapping, observability, kiểm thử chất lượng và kế hoạch rollback như các phần trên.
