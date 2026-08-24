ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS ragflow_chat_id VARCHAR(64);

CREATE UNIQUE INDEX IF NOT EXISTS uk_projects_ragflow_chat_id
    ON projects(ragflow_chat_id)
    WHERE ragflow_chat_id IS NOT NULL AND ragflow_chat_id <> '';
