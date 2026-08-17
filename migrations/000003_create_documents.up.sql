-- Nền ACL/scope tối thiểu để module document luôn kiểm tra quyền theo project.
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY, code VARCHAR(64) NOT NULL, name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '', status VARCHAR(20) NOT NULL DEFAULT 'active',
    owner_id UUID NOT NULL REFERENCES users(id), version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_projects_code_active ON projects (lower(code)) WHERE deleted_at IS NULL;
CREATE TABLE IF NOT EXISTS project_members (
    project_id UUID NOT NULL REFERENCES projects(id), user_id UUID NOT NULL REFERENCES users(id),
    role VARCHAR(20) NOT NULL CHECK (role IN ('owner','editor','viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id,user_id)
);
CREATE TABLE IF NOT EXISTS project_versions (
    id UUID PRIMARY KEY, project_id UUID NOT NULL REFERENCES projects(id), label VARCHAR(100) NOT NULL,
    sequence_no BIGINT NOT NULL, status VARCHAR(20) NOT NULL CHECK (status IN ('draft','published','archived')),
    released_at TIMESTAMPTZ, created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id,id), UNIQUE(project_id,label), UNIQUE(project_id,sequence_no)
);
CREATE TABLE IF NOT EXISTS change_requests (
    id UUID PRIMARY KEY, project_id UUID NOT NULL REFERENCES projects(id), code VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL, status VARCHAR(20) NOT NULL CHECK (status IN ('draft','review','accepted','rejected')),
    sequence_no BIGINT NOT NULL, created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id,id), UNIQUE(project_id,code), UNIQUE(project_id,sequence_no)
);

CREATE TABLE documents (
    id UUID PRIMARY KEY, project_id UUID NOT NULL REFERENCES projects(id), title VARCHAR(255) NOT NULL,
    document_key VARCHAR(255) NOT NULL, description TEXT NOT NULL DEFAULT '', source_type VARCHAR(20) NOT NULL DEFAULT 'upload',
    created_by UUID NOT NULL REFERENCES users(id), version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uk_documents_key_active ON documents(project_id,document_key) WHERE deleted_at IS NULL;
CREATE TABLE document_revisions (
    id UUID PRIMARY KEY, document_id UUID NOT NULL REFERENCES documents(id), project_id UUID NOT NULL REFERENCES projects(id),
    project_version_id UUID, change_request_id UUID, revision_no INT NOT NULL,
    file_name VARCHAR(255) NOT NULL, media_type VARCHAR(100) NOT NULL, size_bytes BIGINT NOT NULL CHECK(size_bytes > 0),
    sha256 CHAR(64) NOT NULL, object_key TEXT NOT NULL UNIQUE, status VARCHAR(20) NOT NULL,
    error_code VARCHAR(100), error_detail_sanitized TEXT, created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((project_version_id IS NOT NULL)::int + (change_request_id IS NOT NULL)::int = 1),
    UNIQUE(document_id,revision_no),
    FOREIGN KEY(project_id,project_version_id) REFERENCES project_versions(project_id,id),
    FOREIGN KEY(project_id,change_request_id) REFERENCES change_requests(project_id,id)
);
CREATE UNIQUE INDEX uk_revisions_version_hash ON document_revisions(project_id, project_version_id, sha256)
    WHERE project_version_id IS NOT NULL AND status <> 'archived';
CREATE UNIQUE INDEX uk_revisions_change_hash ON document_revisions(project_id, change_request_id, sha256)
    WHERE change_request_id IS NOT NULL AND status <> 'archived';
CREATE TABLE ingestion_jobs (
    id UUID PRIMARY KEY, document_revision_id UUID NOT NULL REFERENCES document_revisions(id), status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempt INT NOT NULL DEFAULT 0, max_attempts INT NOT NULL DEFAULT 3, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uk_ingestion_jobs_active ON ingestion_jobs(document_revision_id) WHERE status IN ('pending','running');
CREATE TABLE document_uploads (
    id UUID PRIMARY KEY, project_id UUID NOT NULL REFERENCES projects(id), document_id UUID NOT NULL,
    revision_id UUID NOT NULL, project_version_id UUID, change_request_id UUID, title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '', file_name VARCHAR(255) NOT NULL, media_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL, sha256 CHAR(64) NOT NULL, object_key TEXT NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', created_by UUID NOT NULL REFERENCES users(id), expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ,
    CHECK ((project_version_id IS NOT NULL)::int + (change_request_id IS NOT NULL)::int = 1)
);
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY, topic VARCHAR(100) NOT NULL, aggregate_type VARCHAR(50) NOT NULL, aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL, status VARCHAR(20) NOT NULL DEFAULT 'pending', attempt INT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY, actor_user_id UUID NOT NULL REFERENCES users(id), project_id UUID NOT NULL REFERENCES projects(id),
    action VARCHAR(100) NOT NULL, entity_type VARCHAR(50) NOT NULL, entity_id UUID NOT NULL,
    request_id VARCHAR(100) NOT NULL DEFAULT '', metadata JSONB NOT NULL DEFAULT '{}', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_documents_project ON documents(project_id,updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_revisions_document ON document_revisions(document_id,revision_no DESC);
