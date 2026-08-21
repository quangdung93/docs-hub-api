DROP INDEX IF EXISTS uk_revisions_ragflow_document_id;
CREATE UNIQUE INDEX uk_revisions_ragflow_document_id
    ON document_revisions(ragflow_document_id)
    WHERE ragflow_document_id IS NOT NULL;

DROP INDEX IF EXISTS uk_projects_ragflow_dataset_id;
CREATE UNIQUE INDEX uk_projects_ragflow_dataset_id
    ON projects(ragflow_dataset_id)
    WHERE ragflow_dataset_id IS NOT NULL;
