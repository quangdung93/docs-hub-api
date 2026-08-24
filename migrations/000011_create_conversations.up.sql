CREATE TABLE conversations (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL DEFAULT '',
    active_scope JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_conversations_owner
    ON conversations(project_id, user_id, updated_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE messages (
    id UUID PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    intent VARCHAR(50) NOT NULL DEFAULT '',
    resolved_scope JSONB,
    model VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version VARCHAR(50) NOT NULL DEFAULT '',
    latency_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_conversation
    ON messages(conversation_id, created_at, id);

CREATE TABLE message_citations (
    id UUID PRIMARY KEY,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    chunk_id VARCHAR(255) NOT NULL,
    citation_order INT NOT NULL,
    quoted_text TEXT NOT NULL,
    document_id UUID NOT NULL REFERENCES documents(id),
    document_revision_id UUID NOT NULL REFERENCES document_revisions(id),
    document_title_snapshot VARCHAR(255) NOT NULL,
    document_name_snapshot VARCHAR(255) NOT NULL,
    scope_type VARCHAR(30) NOT NULL,
    scope_label_snapshot VARCHAR(255) NOT NULL,
    line_start INT,
    line_end INT,
    page_start INT,
    page_end INT,
    source_url TEXT NOT NULL,
    UNIQUE(message_id, citation_order)
);

CREATE INDEX idx_message_citations_message
    ON message_citations(message_id, citation_order);
