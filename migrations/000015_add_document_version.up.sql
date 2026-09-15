ALTER TABLE document_revisions
    ADD COLUMN IF NOT EXISTS document_version TEXT NOT NULL DEFAULT '';

ALTER TABLE document_uploads
    ADD COLUMN IF NOT EXISTS document_version TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_revisions_project_document_version_uploaded
    ON document_revisions(project_id, LOWER(document_version), created_at DESC)
    WHERE document_version <> '';
