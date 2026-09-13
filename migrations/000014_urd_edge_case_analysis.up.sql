-- URD v1.2 mục XI: nhận diện tài liệu URD + AI phân tích edge case + tạo
-- phiên bản URD mới. Xem docs/architecture/ADR-0008..0011.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS doc_type VARCHAR(20);

CREATE TABLE IF NOT EXISTS urd_analyses (
    id UUID PRIMARY KEY, document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    document_revision_id UUID NOT NULL REFERENCES document_revisions(id),
    status VARCHAR(20) NOT NULL DEFAULT 'analyzing',
    total_cases INT NOT NULL DEFAULT 0, resolved_cases INT NOT NULL DEFAULT 0,
    created_by UUID NOT NULL REFERENCES users(id),
    error_code VARCHAR(100), error_detail_sanitized TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Chỉ 1 phân tích đang hoạt động (chưa completed/failed) cho mỗi tài liệu —
-- tránh mất dữ liệu đang nhập dở khi bấm phân tích lại.
CREATE UNIQUE INDEX IF NOT EXISTS uk_urd_analyses_active ON urd_analyses(document_id)
    WHERE status IN ('analyzing','awaiting_input');

CREATE TABLE IF NOT EXISTS urd_edge_cases (
    id UUID PRIMARY KEY, analysis_id UUID NOT NULL REFERENCES urd_analyses(id) ON DELETE CASCADE,
    sequence_no INT NOT NULL, description TEXT NOT NULL,
    resolution TEXT NOT NULL DEFAULT '', image_object_key TEXT,
    resolved BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(analysis_id,sequence_no)
);
CREATE INDEX IF NOT EXISTS idx_urd_edge_cases_analysis ON urd_edge_cases(analysis_id);
