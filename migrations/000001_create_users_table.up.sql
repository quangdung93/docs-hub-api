-- Bảng users. Lưu ý:
--   * id là CHAR(36) (UUID string).
--   * roles lưu JSON dạng varchar cho đơn giản (RBAC nâng cao là extension sau).
--   * version phục vụ optimistic lock.
--   * Unique index đặt trên (email, deleted_at) — KHÔNG chỉ (email) — để email
--     đã xóa mềm có thể được tạo lại. Xem ADR-0004 về ngữ nghĩa NULL của MySQL.
CREATE TABLE IF NOT EXISTS users (
    id            CHAR(36)     NOT NULL,
    email         VARCHAR(255) NOT NULL,
    full_name     VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'active',
    roles         VARCHAR(512) NOT NULL DEFAULT '[]',
    version       INT          NOT NULL DEFAULT 1,
    created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at    DATETIME(3)  NULL DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_email_deleted (email, deleted_at),
    KEY idx_users_deleted_at (deleted_at),
    KEY idx_users_status_created (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
