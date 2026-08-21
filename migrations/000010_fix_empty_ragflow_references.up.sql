-- Chuỗi rỗng nghĩa là chưa đồng bộ, không phải một remote ID hợp lệ.
UPDATE projects SET ragflow_dataset_id = NULL WHERE ragflow_dataset_id = '';
UPDATE document_revisions SET ragflow_document_id = NULL WHERE ragflow_document_id = '';

DROP INDEX IF EXISTS uk_projects_ragflow_dataset_id;
CREATE UNIQUE INDEX uk_projects_ragflow_dataset_id
    ON projects(ragflow_dataset_id)
    WHERE ragflow_dataset_id IS NOT NULL AND ragflow_dataset_id <> '';

DROP INDEX IF EXISTS uk_revisions_ragflow_document_id;
CREATE UNIQUE INDEX uk_revisions_ragflow_document_id
    ON document_revisions(ragflow_document_id)
    WHERE ragflow_document_id IS NOT NULL AND ragflow_document_id <> '';
