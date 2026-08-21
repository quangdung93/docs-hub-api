-- Mở rộng bảng project hiện hữu của main cho versioning/document hub.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS code VARCHAR(64);
UPDATE projects SET code = 'project_' || replace(id::text, '-', '') WHERE code IS NULL OR code = '';
ALTER TABLE projects ALTER COLUMN code SET NOT NULL;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE projects ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS settings JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS avatar_key TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS uk_projects_code_active ON projects (lower(code)) WHERE deleted_at IS NULL;

-- Chuẩn hóa project_members nếu database đã từng chạy migration document cũ
-- (schema cũ dùng PK ghép và chưa có id/status/invited_at/joined_at).
ALTER TABLE project_members ADD COLUMN IF NOT EXISTS id UUID;
ALTER TABLE project_members ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE project_members ADD COLUMN IF NOT EXISTS invited_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE project_members ADD COLUMN IF NOT EXISTS joined_at TIMESTAMPTZ;
UPDATE project_members
SET id = md5(project_id::text || ':' || user_id::text)::uuid
WHERE id IS NULL;
UPDATE project_members SET joined_at = COALESCE(joined_at, invited_at) WHERE status = 'active';
ALTER TABLE project_members ALTER COLUMN id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uk_project_members_id ON project_members(id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_project_members_project_user ON project_members(project_id,user_id);

CREATE TABLE IF NOT EXISTS project_versions (
    id UUID PRIMARY KEY, project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    label VARCHAR(100) NOT NULL, sequence_no BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('draft','published','archived')),
    released_at TIMESTAMPTZ, created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id,id), UNIQUE(project_id,label), UNIQUE(project_id,sequence_no)
);
CREATE TABLE IF NOT EXISTS change_requests (
    id UUID PRIMARY KEY, project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    code VARCHAR(100) NOT NULL, title VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('draft','review','accepted','rejected')),
    sequence_no BIGINT NOT NULL, created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id,id), UNIQUE(project_id,code), UNIQUE(project_id,sequence_no)
);
CREATE TABLE IF NOT EXISTS documents (
    id UUID PRIMARY KEY, project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL, document_key VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '', source_type VARCHAR(20) NOT NULL DEFAULT 'upload',
    created_by UUID NOT NULL REFERENCES users(id), version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_documents_key_active ON documents(project_id,document_key) WHERE deleted_at IS NULL;
CREATE TABLE IF NOT EXISTS document_revisions (
    id UUID PRIMARY KEY, document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    project_version_id UUID, change_request_id UUID, revision_no INT NOT NULL,
    file_name VARCHAR(255) NOT NULL, media_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL CHECK(size_bytes > 0), sha256 CHAR(64) NOT NULL,
    object_key TEXT NOT NULL UNIQUE, status VARCHAR(20) NOT NULL,
    error_code VARCHAR(100), error_detail_sanitized TEXT,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((project_version_id IS NOT NULL)::int + (change_request_id IS NOT NULL)::int = 1),
    UNIQUE(document_id,revision_no),
    FOREIGN KEY(project_id,project_version_id) REFERENCES project_versions(project_id,id),
    FOREIGN KEY(project_id,change_request_id) REFERENCES change_requests(project_id,id)
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_revisions_version_hash ON document_revisions(project_id, project_version_id, sha256)
    WHERE project_version_id IS NOT NULL AND status <> 'archived';
CREATE UNIQUE INDEX IF NOT EXISTS uk_revisions_change_hash ON document_revisions(project_id, change_request_id, sha256)
    WHERE change_request_id IS NOT NULL AND status <> 'archived';
CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id UUID PRIMARY KEY, document_revision_id UUID NOT NULL REFERENCES document_revisions(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', attempt INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ingestion_jobs_active ON ingestion_jobs(document_revision_id) WHERE status IN ('pending','running');
CREATE TABLE IF NOT EXISTS document_uploads (
    id UUID PRIMARY KEY, project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_id UUID NOT NULL, revision_id UUID NOT NULL,
    project_version_id UUID, change_request_id UUID, title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '', file_name VARCHAR(255) NOT NULL,
    media_type VARCHAR(100) NOT NULL, size_bytes BIGINT NOT NULL, sha256 CHAR(64) NOT NULL,
    object_key TEXT NOT NULL UNIQUE, status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_by UUID NOT NULL REFERENCES users(id), expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ,
    CHECK ((project_version_id IS NOT NULL)::int + (change_request_id IS NOT NULL)::int = 1)
);
CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY, topic VARCHAR(100) NOT NULL, aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id UUID NOT NULL, payload JSONB NOT NULL, status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempt INT NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY, actor_user_id UUID NOT NULL REFERENCES users(id),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    action VARCHAR(100) NOT NULL, entity_type VARCHAR(50) NOT NULL, entity_id UUID NOT NULL,
    request_id VARCHAR(100) NOT NULL DEFAULT '', metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_documents_project ON documents(project_id,updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_revisions_document ON document_revisions(document_id,revision_no DESC);
