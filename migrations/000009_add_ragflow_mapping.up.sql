ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS ragflow_dataset_id VARCHAR(64),
    ADD COLUMN IF NOT EXISTS ragflow_sync_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS ragflow_last_error TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uk_projects_ragflow_dataset_id
    ON projects(ragflow_dataset_id)
    WHERE ragflow_dataset_id IS NOT NULL AND ragflow_dataset_id <> '';

ALTER TABLE document_revisions
    ADD COLUMN IF NOT EXISTS ragflow_document_id VARCHAR(64),
    ADD COLUMN IF NOT EXISTS ragflow_sync_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS ragflow_last_error TEXT,
    ADD COLUMN IF NOT EXISTS ragflow_synced_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS uk_revisions_ragflow_document_id
    ON document_revisions(ragflow_document_id)
    WHERE ragflow_document_id IS NOT NULL AND ragflow_document_id <> '';
