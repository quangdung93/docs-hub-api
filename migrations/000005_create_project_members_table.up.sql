-- Bảng project_members (PostgreSQL).
--   * Mỗi (project_id, user_id) là duy nhất — 1 user chỉ có 1 vai trò/lời mời
--     trên mỗi dự án.
--   * status: 'pending' khi vừa được mời, 'active' sau khi accept (joined_at được set).
--   * ON DELETE CASCADE trên project_id: xóa cứng project sẽ tự dọn thành viên.
CREATE TABLE IF NOT EXISTS project_members (
    id         UUID        NOT NULL,
    project_id UUID        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users (id),
    role       TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'pending',
    invited_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    joined_at  TIMESTAMPTZ,
    PRIMARY KEY (id),
    UNIQUE (project_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_project_members_project_id ON project_members (project_id);
CREATE INDEX IF NOT EXISTS idx_project_members_user_id ON project_members (user_id);