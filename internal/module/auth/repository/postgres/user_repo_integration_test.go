//go:build integration

// Integration test cho auth user repository. Chạy: make test-integration (cần Docker).
//
// Lý do có file này: truy vấn của auth dùng Table("users") với tên bảng dạng
// chuỗi nên GORM KHÔNG tự áp scope soft-delete. Bug đã xảy ra thật trên
// production — tài khoản bị xóa vẫn đăng nhập được bình thường. Test ở đây chốt
// hành vi đó lại; unit test với mock không phát hiện được vì lỗi nằm ở SQL.
package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	authpg "github.com/quangdung93/docs-hub-api/internal/module/auth/repository/postgres"
)

const createUsersTable = `
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY,
    email         VARCHAR(255) NOT NULL,
    full_name     VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'active',
    roles         VARCHAR(512) NOT NULL DEFAULT '[]',
    version       INT          NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
)`

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("cần TEST_POSTGRES_DSN để chạy test này")
	}
	db, err := postgres.New(postgres.Config{DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2}, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, db.Exec(createUsersTable).Error)
	require.NoError(t, db.Exec(`TRUNCATE TABLE users CASCADE`).Error)
	return db
}

// insertUser chèn thẳng vào bảng; deletedAt khác nil nghĩa là đã xóa mềm.
func insertUser(t *testing.T, db *gorm.DB, id uuid.UUID, email string, deletedAt *time.Time) {
	t.Helper()
	const sql = `INSERT INTO users(id,email,full_name,password_hash,roles,deleted_at)
		VALUES(?,?,?,?,?,?)`
	require.NoError(t, db.Exec(sql, id.String(), email, "Người dùng", "hash", `["admin"]`, deletedAt).Error)
}

// TestFindByUsername_BoQuaUserDaXoa chốt lỗ hổng đã gặp trên production:
// tài khoản xóa mềm KHÔNG được đăng nhập nữa.
func TestFindByUsername_BoQuaUserDaXoa(t *testing.T) {
	db := setupDB(t)
	repo := authpg.NewUserRepository(db)
	ctx := context.Background()
	now := time.Now()

	activeID, deletedID := uuid.New(), uuid.New()
	insertUser(t, db, activeID, "active@docshub.io.vn", nil)
	insertUser(t, db, deletedID, "deleted@docshub.io.vn", &now)

	t.Run("user còn hoạt động thì tìm thấy", func(t *testing.T) {
		got, err := repo.FindByUsername(ctx, "active@docshub.io.vn")
		require.NoError(t, err)
		require.Equal(t, activeID, got.ID)
	})

	t.Run("user đã xóa mềm thì KHÔNG tìm thấy", func(t *testing.T) {
		_, err := repo.FindByUsername(ctx, "deleted@docshub.io.vn")
		require.Error(t, err, "user đã xóa mà vẫn tìm thấy -> đăng nhập được -> xóa tài khoản vô tác dụng")
	})
}

// TestFindByID_BoQuaUserDaXoa: cùng lỗ hổng, ở đường /auth/me và refresh token.
func TestFindByID_BoQuaUserDaXoa(t *testing.T) {
	db := setupDB(t)
	repo := authpg.NewUserRepository(db)
	ctx := context.Background()
	now := time.Now()

	activeID, deletedID := uuid.New(), uuid.New()
	insertUser(t, db, activeID, "active2@docshub.io.vn", nil)
	insertUser(t, db, deletedID, "deleted2@docshub.io.vn", &now)

	t.Run("user còn hoạt động thì tìm thấy", func(t *testing.T) {
		got, err := repo.FindByID(ctx, activeID.String())
		require.NoError(t, err)
		require.Equal(t, activeID, got.ID)
	})

	t.Run("user đã xóa mềm thì KHÔNG tìm thấy", func(t *testing.T) {
		_, err := repo.FindByID(ctx, deletedID.String())
		require.Error(t, err, "user đã xóa vẫn gọi được /auth/me và gia hạn token")
	})
}
