-- Bảng users (PostgreSQL). Lưu ý:
--   * id là UUID (kiểu native của Postgres).
--   * roles lưu JSON dạng varchar cho đơn giản (RBAC nâng cao là extension sau).
--   * version phục vụ optimistic lock.
--   * Unique dùng PARTIAL INDEX `... WHERE deleted_at IS NULL` — sạch hơn hẳn
--     composite (email, deleted_at) của MySQL: email đã xóa mềm được tạo lại,
--     nhưng chỉ 1 bản ghi SỐNG mỗi email. Xem ADR-0004 + ADR-0006.
CREATE TABLE IF NOT EXISTS users (
    id            UUID         NOT NULL,
    email         VARCHAR(255) NOT NULL,
    full_name     VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'active',
    roles         VARCHAR(512) NOT NULL DEFAULT '[]',
    version       INT          NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    PRIMARY KEY (id)
);

-- Chỉ 1 email SỐNG (chưa xóa mềm) — partial unique index.
CREATE UNIQUE INDEX IF NOT EXISTS uk_users_email_active ON users (email) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at);
CREATE INDEX IF NOT EXISTS idx_users_status_created ON users (status, created_at);
