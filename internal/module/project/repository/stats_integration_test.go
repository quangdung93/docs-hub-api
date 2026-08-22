//go:build integration

// Integration test cho Stats của project repository. Chạy: make test-integration.
//
// Lý do có file này: Stats dùng SQL thô nên GORM KHÔNG tự áp scope soft-delete.
// Bug đã xảy ra thật trên production — xóa hết tài liệu, danh sách trả về rỗng
// nhưng document_count vẫn báo 5. Unit test với mock không phát hiện được vì lỗi
// nằm nguyên trong câu SQL.
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

	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/project/repository"
)

// Schema tối thiểu, chỉ đủ cột mà Stats đụng tới, KHÔNG khóa ngoại.
//
// Test này nhắm vào database sạch (service postgres của CI, hoặc testcontainer).
// Cố ý không TRUNCATE và không DROP: nếu ai đó lỡ trỏ TEST_POSTGRES_DSN vào
// database thật thì test phải hỏng ồn ào chứ không được xóa dữ liệu của họ.
var statsSchema = []string{
	`CREATE TABLE IF NOT EXISTS projects (
		id UUID PRIMARY KEY
	)`,
	`CREATE TABLE IF NOT EXISTS documents (
		id UUID PRIMARY KEY,
		project_id UUID NOT NULL,
		deleted_at TIMESTAMPTZ
	)`,
	`CREATE TABLE IF NOT EXISTS project_members (
		id UUID PRIMARY KEY,
		project_id UUID NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS document_chunks (
		id UUID PRIMARY KEY,
		project_id UUID NOT NULL
	)`,
}

func setupStatsDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = startStatsPostgres(t)
	}
	db, err := postgres.New(postgres.Config{DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2}, zap.NewNop())
	require.NoError(t, err)
	// Mỗi phần tử phải là MỘT câu lệnh: pgx chạy qua extended protocol nên
	// Postgres từ chối chuỗi nhiều lệnh (SQLSTATE 42601).
	for _, stmt := range statsSchema {
		require.NoError(t, db.Exec(stmt).Error)
	}
	return db
}

func startStatsPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "pgvector/pgvector:pg16",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "app",
				"POSTGRES_PASSWORD": "app_password",
				"POSTGRES_DB":       "document_hub",
			},
			WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, "5432")
	require.NoError(t, err)
	return fmt.Sprintf(
		"host=%s port=%s user=app password=app_password dbname=document_hub sslmode=disable TimeZone=UTC",
		host, port.Port(),
	)
}

// TestStats_KhongDemTaiLieuDaXoaMem chốt lỗi đã gặp trên production.
func TestStats_KhongDemTaiLieuDaXoaMem(t *testing.T) {
	db := setupStatsDB(t)
	repo := repository.NewProjectRepository(db)
	ctx := context.Background()
	now := time.Now()

	// UUID riêng cho từng lần chạy để test lặp lại được trên cùng một Postgres
	// mà không đụng dữ liệu của lần trước.
	pid := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO projects(id) VALUES(?)`, pid.String()).Error)

	insertDoc := func(deletedAt *time.Time) {
		require.NoError(t, db.Exec(
			`INSERT INTO documents(id,project_id,deleted_at) VALUES(?,?,?)`,
			uuid.New().String(), pid.String(), deletedAt,
		).Error)
	}
	insertDoc(nil)
	insertDoc(nil)
	insertDoc(&now) // đã xóa mềm
	insertDoc(&now) // đã xóa mềm

	require.NoError(t, db.Exec(`INSERT INTO project_members(id,project_id) VALUES(?,?)`,
		uuid.New().String(), pid.String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO document_chunks(id,project_id) VALUES(?,?)`,
		uuid.New().String(), pid.String()).Error)

	stats, err := repo.Stats(ctx, []uuid.UUID{pid})
	require.NoError(t, err)
	require.Contains(t, stats, pid)

	got := stats[pid]
	require.Equal(t, int64(2), got.DocumentCount,
		"chỉ được đếm tài liệu còn sống; đếm cả bản ghi xóa mềm thì danh sách rỗng mà counter vẫn dương")
	require.Equal(t, int64(1), got.MemberCount)
	require.Equal(t, int64(1), got.ChunkCount)
}

// TestStats_DemDungTungDuAn: Stats nhận nhiều id một lượt, mỗi dự án phải ra
// đúng số của mình chứ không lẫn sang dự án khác.
func TestStats_DemDungTungDuAn(t *testing.T) {
	db := setupStatsDB(t)
	repo := repository.NewProjectRepository(db)
	ctx := context.Background()
	now := time.Now()

	pidA, pidB := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{pidA, pidB} {
		require.NoError(t, db.Exec(`INSERT INTO projects(id) VALUES(?)`, id.String()).Error)
	}

	insertDoc := func(pid uuid.UUID, deletedAt *time.Time) {
		require.NoError(t, db.Exec(
			`INSERT INTO documents(id,project_id,deleted_at) VALUES(?,?,?)`,
			uuid.New().String(), pid.String(), deletedAt,
		).Error)
	}
	insertDoc(pidA, nil)
	insertDoc(pidB, nil)
	insertDoc(pidB, nil)
	insertDoc(pidB, &now)

	stats, err := repo.Stats(ctx, []uuid.UUID{pidA, pidB})
	require.NoError(t, err)
	require.Len(t, stats, 2)
	require.Equal(t, int64(1), stats[pidA].DocumentCount)
	require.Equal(t, int64(2), stats[pidB].DocumentCount)
}

// TestStats_DuAnRong: dự án chưa có gì phải trả 0, không phải thiếu key.
func TestStats_DuAnRong(t *testing.T) {
	db := setupStatsDB(t)
	repo := repository.NewProjectRepository(db)

	pid := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO projects(id) VALUES(?)`, pid.String()).Error)

	stats, err := repo.Stats(context.Background(), []uuid.UUID{pid})
	require.NoError(t, err)
	require.Contains(t, stats, pid)
	require.Equal(t, int64(0), stats[pid].DocumentCount)
	require.Equal(t, int64(0), stats[pid].MemberCount)
	require.Equal(t, int64(0), stats[pid].ChunkCount)
}
