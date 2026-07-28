-- Bật extension pgvector — điểm nối (seam) cho hệ RAG (xem ADR-0006).
-- Module ingestion/chat tương lai sẽ tạo bảng document_chunks với cột
-- `embedding vector(N)` + index HNSW/IVFFlat. Ở đây chỉ bật extension để hạ tầng
-- sẵn sàng; chưa tạo bảng vì chưa có module dùng (tránh dead schema).
CREATE EXTENSION IF NOT EXISTS vector;
