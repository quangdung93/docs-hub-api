//go:build integration

// Integration test cho user repository. Chạy: make test-integration (cần Docker).
//
// Tự phát hiện DSN: nếu có env TEST_POSTGRES_DSN (ví dụ trong CI với service
// postgres) thì dùng luôn; ngược lại tự spawn container Postgres (pgvector) bằng
// testcontainers.
package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/quangdung393/docs-hub-api/internal/common/pagination"
	"github.com/quangdung393/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung393/docs-hub-api/internal/module/user/domain"
	"github.com/quangdung393/docs-hub-api/internal/module/user/repository"
)

// paginationQuery là helper tạo query đã normalize cho test.
func paginationQuery(page, limit int) pagination.Query {
	return pagination.Query{Page: page, Limit: limit}.Normalize()
}

// createUsersTable dựng schema tối thiểu cho test (khớp migration Postgres):
// dùng UNIQUE partial index thay vì composite (email, deleted_at).
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
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_users_email_active ON users (email) WHERE deleted_at IS NULL;`

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = startPostgresContainer(t)
	}
	db, err := postgres.New(postgres.Config{DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2}, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, db.Exec(createUsersTable).Error)
	return db
}

func startPostgresContainer(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "pgvector/pgvector:pg16",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "app",
			"POSTGRES_PASSWORD": "app_password",
			"POSTGRES_DB":       "document_hub",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(2 * time.Minute),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, "5432")
	require.NoError(t, err)

	return fmt.Sprintf("host=%s port=%s user=app password=app_password dbname=document_hub sslmode=disable TimeZone=UTC",
		host, port.Port())
}

func TestUserRepository_CRUD(t *testing.T) {
	db := setupDB(t)
	repo := repository.New(db)
	ctx := context.Background()

	u := domain.NewUser("crud@b.com", "CRUD User", "hash", []string{"user"})
	require.NoError(t, repo.Create(ctx, u))

	got, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "crud@b.com", got.Email)
	require.Equal(t, 1, got.Version)
}

func TestUserRepository_OptimisticLock(t *testing.T) {
	db := setupDB(t)
	repo := repository.New(db)
	ctx := context.Background()

	u := domain.NewUser("lock@b.com", "Lock User", "hash", nil)
	require.NoError(t, repo.Create(ctx, u))

	// Update lần 1 với version đúng (1) -> thành công, version thành 2.
	u.FullName = "Đổi lần 1"
	u.Version = 1
	require.NoError(t, repo.Update(ctx, u))

	// Update lần 2 với version CŨ (1) -> không tác động dòng nào.
	u.FullName = "Đổi lần 2"
	u.Version = 1
	err := repo.Update(ctx, u)
	require.ErrorIs(t, err, domain.ErrNoRowsAffected)
}

func TestUserRepository_SoftDeleteThenRecreateEmail(t *testing.T) {
	db := setupDB(t)
	repo := repository.New(db)
	ctx := context.Background()

	u := domain.NewUser("soft@b.com", "Soft User", "hash", nil)
	require.NoError(t, repo.Create(ctx, u))
	require.NoError(t, repo.SoftDelete(ctx, u.ID, 1))

	// Sau xóa mềm: FindByID không thấy (deleted_at IS NULL scope).
	_, err := repo.FindByID(ctx, u.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Nhờ unique (email, deleted_at), có thể tạo lại email đã xóa mềm.
	u2 := domain.NewUser("soft@b.com", "Soft User 2", "hash", nil)
	require.NoError(t, repo.Create(ctx, u2))
}

func TestUserRepository_DuplicateEmail(t *testing.T) {
	db := setupDB(t)
	repo := repository.New(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, domain.NewUser("dup@b.com", "A", "h", nil)))

	err := repo.Create(ctx, domain.NewUser("dup@b.com", "B", "h", nil))
	require.ErrorIs(t, err, domain.ErrDuplicate)
}

func TestUserRepository_ListPagination(t *testing.T) {
	db := setupDB(t)
	repo := repository.New(db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		email := uuid.NewString() + "@b.com"
		require.NoError(t, repo.Create(ctx, domain.NewUser(email, "User", "h", nil)))
	}

	users, total, err := repo.List(ctx, domain.Filter{}, paginationQuery(1, 2))
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, int64(5))
	require.Len(t, users, 2)
}
