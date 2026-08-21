-- Bảng projects (PostgreSQL).
--   * id là UUID (sinh ở tầng ứng dụng, giống bảng users).
--   * settings lưu JSONB (model, top_k, chunk_size, allowed_formats) cho cấu hình RAG.
--   * Không có version/deleted_at: xóa dự án là HARD DELETE (xem ADR liên quan).
CREATE TABLE IF NOT EXISTS projects (
    id          UUID        NOT NULL,
    owner_id    UUID        REFERENCES users (id),
    name        TEXT        NOT NULL,
    description TEXT,
    status      TEXT        NOT NULL DEFAULT 'active',
    settings    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_projects_owner_id ON projects (owner_id);