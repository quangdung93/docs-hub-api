//go:build integration

// Integration test cho report repository. Chạy: make test-integration (cần Docker).
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
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/report/repository"
)

// schemaStatements dựng schema tối thiểu khớp migration thật (000013 +
// các cột ragflow_* của 000009/000012) — đủ để test repository, không cần
// toàn bộ migration chain.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY, email VARCHAR(255) NOT NULL, deleted_at TIMESTAMPTZ)`,
	`CREATE TABLE IF NOT EXISTS projects (
		id UUID PRIMARY KEY, name VARCHAR(255) NOT NULL, code VARCHAR(50) NOT NULL,
		ragflow_dataset_id VARCHAR(64), ragflow_chat_id VARCHAR(64),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), deleted_at TIMESTAMPTZ)`,
	`CREATE TABLE IF NOT EXISTS project_members (
		project_id UUID NOT NULL, user_id UUID NOT NULL, role VARCHAR(20) NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS project_reports (
		id UUID PRIMARY KEY, project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		report_type VARCHAR(20) NOT NULL CHECK (report_type IN ('uat','planning','testcase')),
		format VARCHAR(10) NOT NULL CHECK (format IN ('xlsx','pdf')),
		file_path VARCHAR(500) NOT NULL, generated_by UUID NOT NULL REFERENCES users(id),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
	`CREATE TABLE IF NOT EXISTS project_report_items (
		id UUID PRIMARY KEY,
		project_report_id UUID NOT NULL REFERENCES project_reports(id) ON DELETE CASCADE,
		source_ref VARCHAR(255) NOT NULL DEFAULT '', title VARCHAR(500) NOT NULL,
		detail TEXT NOT NULL, status VARCHAR(50) NOT NULL DEFAULT '', sequence_no INT NOT NULL)`,
}

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = startPostgresContainer(t)
	}
	db, err := postgres.New(postgres.Config{DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2}, zap.NewNop())
	require.NoError(t, err)
	for _, stmt := range schemaStatements {
		require.NoError(t, db.Exec(stmt).Error)
	}
	require.NoError(t, db.Exec(`TRUNCATE TABLE project_report_items, project_reports,
		project_members, projects, users`).Error)
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

func seedProject(t *testing.T, db *gorm.DB, projectID, userID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO users (id, email) VALUES (?, ?)`,
		userID, userID.String()+"@example.com").Error)
	require.NoError(t, db.Exec(`INSERT INTO projects (id, name, code) VALUES (?, ?, ?)`,
		projectID, "Demo Project", "DEMO").Error)
}

func TestReportRepository_CreateVaListHistory(t *testing.T) {
	db := setupDB(t)
	repo := repository.New(db)
	ctx := context.Background()

	projectID, userID := uuid.New(), uuid.New()
	seedProject(t, db, projectID, userID)

	report := domain.Report{
		ID: uuid.New(), ProjectID: projectID, ReportType: domain.ReportTypeUAT,
		Format: domain.FormatXLSX, FileKey: "reports/x/y.xlsx", GeneratedBy: userID,
		CreatedAt: time.Now().UTC(),
	}
	items := []domain.ReportItem{
		{ID: uuid.New(), ReportID: report.ID, Title: "Đăng nhập", Detail: "chi tiết", SequenceNo: 1},
		{ID: uuid.New(), ReportID: report.ID, Title: "Đăng xuất", Detail: "chi tiết", SequenceNo: 2},
	}
	require.NoError(t, repo.Create(ctx, report, items))

	var itemCount int64
	require.NoError(t, db.Table("project_report_items").
		Where("project_report_id=?", report.ID).Count(&itemCount).Error)
	require.Equal(t, int64(2), itemCount)

	history, total, err := repo.ListHistory(ctx, projectID, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, history, 1)
	require.Equal(t, report.ID, history[0].ID)
	require.Equal(t, "reports/x/y.xlsx", history[0].FileKey)
}

func TestReportRepository_ListHistory_ThuTuMoiNhatTruoc(t *testing.T) {
	db := setupDB(t)
	repo := repository.New(db)
	ctx := context.Background()

	projectID, userID := uuid.New(), uuid.New()
	seedProject(t, db, projectID, userID)

	older := domain.Report{
		ID: uuid.New(), ProjectID: projectID, ReportType: domain.ReportTypeUAT,
		Format: domain.FormatXLSX, FileKey: "a.xlsx", GeneratedBy: userID,
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	}
	newer := domain.Report{
		ID: uuid.New(), ProjectID: projectID, ReportType: domain.ReportTypeUAT,
		Format: domain.FormatPDF, FileKey: "b.pdf", GeneratedBy: userID,
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, older, nil))
	require.NoError(t, repo.Create(ctx, newer, nil))

	history, total, err := repo.ListHistory(ctx, projectID, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, history, 2)
	require.Equal(t, newer.ID, history[0].ID, "report mới nhất phải đứng đầu")
	require.Equal(t, older.ID, history[1].ID)
}

func TestReportRepository_RAGFlowMapping(t *testing.T) {
	db := setupDB(t)
	repo := repository.New(db)
	ctx := context.Background()

	projectID, userID := uuid.New(), uuid.New()
	seedProject(t, db, projectID, userID)
	require.NoError(t, db.Exec(`UPDATE projects SET ragflow_dataset_id=? WHERE id=?`, "ds-1", projectID).Error)

	datasetID, err := repo.RAGFlowDatasetID(ctx, projectID)
	require.NoError(t, err)
	require.Equal(t, "ds-1", datasetID)

	chatID, err := repo.RAGFlowChatID(ctx, projectID)
	require.NoError(t, err)
	require.Empty(t, chatID)

	saved, err := repo.SaveRAGFlowChatID(ctx, projectID, "chat-1")
	require.NoError(t, err)
	require.Equal(t, "chat-1", saved)

	// Lần 2 với chatID khác — cột đã có giá trị nên giữ nguyên (idempotent).
	saved2, err := repo.SaveRAGFlowChatID(ctx, projectID, "chat-2")
	require.NoError(t, err)
	require.Equal(t, "chat-1", saved2)
}

func TestReportRepository_MemberRoleVaProjectMeta(t *testing.T) {
	db := setupDB(t)
	repo := repository.New(db)
	ctx := context.Background()

	projectID, userID := uuid.New(), uuid.New()
	seedProject(t, db, projectID, userID)
	require.NoError(t, db.Exec(`INSERT INTO project_members (project_id, user_id, role) VALUES (?, ?, ?)`,
		projectID, userID, "editor").Error)

	role, err := repo.MemberRole(ctx, projectID, userID)
	require.NoError(t, err)
	require.Equal(t, "editor", role)

	name, code, err := repo.ProjectMeta(ctx, projectID)
	require.NoError(t, err)
	require.Equal(t, "Demo Project", name)
	require.Equal(t, "DEMO", code)
}
