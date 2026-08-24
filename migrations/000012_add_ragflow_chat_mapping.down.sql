DROP INDEX IF EXISTS uk_projects_ragflow_chat_id;

ALTER TABLE projects
    DROP COLUMN IF EXISTS ragflow_chat_id;
