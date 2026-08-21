DROP INDEX IF EXISTS uk_revisions_ragflow_document_id;

ALTER TABLE document_revisions
    DROP COLUMN IF EXISTS ragflow_synced_at,
    DROP COLUMN IF EXISTS ragflow_last_error,
    DROP COLUMN IF EXISTS ragflow_sync_status,
    DROP COLUMN IF EXISTS ragflow_document_id;

DROP INDEX IF EXISTS uk_projects_ragflow_dataset_id;

ALTER TABLE projects
    DROP COLUMN IF EXISTS ragflow_last_error,
    DROP COLUMN IF EXISTS ragflow_sync_status,
    DROP COLUMN IF EXISTS ragflow_dataset_id;
