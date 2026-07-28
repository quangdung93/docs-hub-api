# ADR-0006: Kiến trúc RAG & chọn PostgreSQL + pgvector

- Trạng thái: Được chấp nhận
- Ngày: 2026-07-28
- Liên quan: thay thế một phần [ADR-0004](./ADR-0004-migrations-and-soft-delete.md)

## Bối cảnh
`docs-hub-api` là **hệ hỏi đáp tài liệu doanh nghiệp (RAG)**, không phải CRUD thuần. Người dùng hỏi bằng ngôn ngữ tự nhiên; hệ truy hồi đoạn tài liệu liên quan rồi để LLM sinh câu trả lời kèm trích dẫn.

Kiến trúc đề xuất ban đầu vẽ một chuỗi dọc `Vector DB → Elasticsearch → PostgreSQL`. Đánh giá cho thấy: pattern khả thi, nhưng (1) topology sai — 3 store là **song song** không phải tuần tự, (2) thiếu **luồng ingestion**, (3) thiếu **object storage** và **lọc theo quyền**.

## Quyết định

### 1. Hai luồng, không phải một chuỗi
- **Ingestion (bất đồng bộ):** upload → `docs-hub-api` lưu file gốc vào **MinIO**, metadata vào **PostgreSQL**, đẩy job qua **RabbitMQ** → **Ingestion Worker (Python)**: parse → chunk → embed → ghi **pgvector** + **Elasticsearch**.
- **Query (đồng bộ):** UI → `docs-hub-api` (JWT + nạp ACL) → **RAG Service (Python)**: embed câu hỏi → hybrid retrieve (**lọc theo quyền**) → rerank → prompt → **Azure OpenAI** → trả lời + trích dẫn.

### 2. Relational DB = PostgreSQL 16 + pgvector (thay MySQL)
Lý do đổi từ MySQL:
- **pgvector** biến PostgreSQL thành vector store luôn → **bỏ được 1 hệ** ở giai đoạn đầu (không cần Qdrant/Milvus riêng).
- **Partial unique index** `UNIQUE(email) WHERE deleted_at IS NULL` — giải bài toán unique-khi-soft-delete sạch hơn hẳn composite `(email, deleted_at)` của MySQL (xem ADR-0004).
- Full-text search (`tsvector`/GIN), JSONB, và **Row-Level Security** cho multi-tenant — đều mạnh hơn MySQL cho domain tài liệu.

### 3. Store: gộp còn 2 ở giai đoạn đầu
| Store | Vai trò | Trạng thái |
|---|---|---|
| PostgreSQL + pgvector | Metadata + ACL + **semantic search** | Dùng |
| Elasticsearch | **Keyword/BM25** cho hybrid search | Dùng |
| Vector DB riêng | Semantic chuyên dụng quy mô lớn | **Hoãn** — chỉ thêm khi pgvector chạm trần |

### 4. Tách RAG Service (Python)
Go (`docs-hub-api`) làm **API/BFF + auth + metadata + điều phối ingestion**; RAG Service viết **Python** (parser/embed/rerank trưởng thành), gọi qua HTTP/gRPC.

### 5. LLM & embedding: private
Tài liệu nội bộ **không** gửi ra OpenAI public. Dùng **Azure OpenAI (private endpoint)** hoặc **self-host** (bge-m3/e5) cho embedding.

## Thay đổi trong repo (đã thực hiện)
- `internal/infrastructure/database/postgres/` thay `.../mysql/` (driver `gorm.io/driver/postgres`, errmap SQLSTATE `23505`).
- `config.PostgresConfig` (host/port 5432/ssl_mode), DSN key-value + `postgres://` cho migrate.
- Migration `000001` DDL Postgres (UUID, TIMESTAMPTZ, partial unique index); `000002` bật extension `vector`.
- `docker-compose` dùng ảnh `pgvector/pgvector:pg16`; CI + integration test dùng Postgres.

## Điểm nối cho RAG (chưa code — tránh dead schema)
- Extension `vector` đã bật. Module `ingestion`/`chat` tương lai tạo bảng `document_chunks(embedding vector(N), ...)` + index HNSW.
- `port.ObjectStore` (MinIO), `port.Publisher` (RabbitMQ), JWT+RBAC (ACL) đã sẵn.

## Rủi ro (nhắc lại, cần chốt sớm)
1. **Permission-aware retrieval** — nhúng ACL vào chunk/index từ đầu (như `tenant_id`). Rủi ro #1: lộ tài liệu mật.
2. **Data residency** — private LLM/embedding.
3. **Reranker + citations + RAG eval** — chất lượng câu trả lời.
4. **Re-index theo version** khi tài liệu đổi (tận dụng optimistic lock đã có).

## Hệ quả
- Bỏ phụ thuộc MySQL; thêm `gorm.io/driver/postgres`, `jackc/pgx/v5`.
- ADR-0004 Quyết định 2 (composite unique) được thay bằng partial index; Quyết định 1 giữ nguyên.
- Sơ đồ kiến trúc đầy đủ: xem artifact review kèm theo.
