ALTER TABLE document_revisions ADD COLUMN IF NOT EXISTS embedding_model VARCHAR(255);
ALTER TABLE document_revisions ADD COLUMN IF NOT EXISTS parser_version VARCHAR(50);
ALTER TABLE document_revisions ADD COLUMN IF NOT EXISTS canonical_text_key TEXT;

CREATE TABLE document_chunks (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    document_id UUID NOT NULL REFERENCES documents(id),
    document_revision_id UUID NOT NULL REFERENCES document_revisions(id) ON DELETE CASCADE,
    project_version_id UUID,
    change_request_id UUID,
    ordinal INT NOT NULL,
    content TEXT NOT NULL,
    content_tsv TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED,
    embedding VECTOR NOT NULL,
    line_start INT NOT NULL,
    line_end INT NOT NULL,
    token_count INT NOT NULL,
    content_hash CHAR(64) NOT NULL,
    embedding_model VARCHAR(255) NOT NULL,
    embedding_dimension INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((project_version_id IS NOT NULL)::int + (change_request_id IS NOT NULL)::int = 1),
    UNIQUE(document_revision_id, ordinal)
);
CREATE INDEX idx_chunks_project_scope ON document_chunks(project_id, project_version_id, change_request_id);
CREATE INDEX idx_chunks_revision ON document_chunks(document_revision_id);
CREATE INDEX idx_chunks_tsv ON document_chunks USING GIN(content_tsv);
