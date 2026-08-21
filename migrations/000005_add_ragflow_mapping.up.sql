ALTER TABLE projects
    ADD COLUMN ragflow_dataset_id VARCHAR(64),
    ADD COLUMN ragflow_sync_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    ADD COLUMN ragflow_last_error TEXT;

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
