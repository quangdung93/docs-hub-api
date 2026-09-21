DROP INDEX IF EXISTS idx_revisions_project_document_version_uploaded;

ALTER TABLE document_uploads DROP COLUMN IF EXISTS document_version;
ALTER TABLE document_revisions DROP COLUMN IF EXISTS document_version;
