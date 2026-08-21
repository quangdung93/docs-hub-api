DROP TABLE IF EXISTS document_chunks;
ALTER TABLE document_revisions DROP COLUMN IF EXISTS canonical_text_key;
ALTER TABLE document_revisions DROP COLUMN IF EXISTS parser_version;
ALTER TABLE document_revisions DROP COLUMN IF EXISTS embedding_model;
