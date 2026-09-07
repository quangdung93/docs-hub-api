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

// schemaStatements dựng schema tối thiểu cho test repository. `make
// test-integration` chạy TOÀN BỘ package integration test tuần tự (-p 1) trên
// CÙNG MỘT Postgres trống (xem TEST_POSTGRES_DSN trong ci.yml) — users/
// projects/project_members là bảng DÙNG CHUNG với auth/user/project, package
// nào chạy trước (theo thứ tự alphabet: auth < project < report < user)
// quyết định ai tạo bảng gốc trước. Vì vậy:
//   - CHỈ ALTER TABLE ADD COLUMN IF NOT EXISTS cho các bảng dùng chung, không
//     bao giờ CREATE với cột NOT NULL cố định — nếu package khác đã tạo bảng
//     với ít cột hơn (vd stats_integration_test.go chỉ tạo `projects(id)`),
//     CREATE IF NOT EXISTS của mình sẽ là no-op và INSERT sẽ lỗi "column does
//     not exist" nếu cột mình cần chưa tồn tại.
//   - KHÔNG FK từ project_reports sang users/projects — lỗi thật đã xảy ra:
//     FK khiến `TRUNCATE TABLE users` ở package user báo "cannot truncate a
//     table referenced in a foreign key constraint". 2 bảng của CHÍNH module
//     report thì vẫn giữ FK nội bộ (project_report_items -> project_reports),
//     an toàn vì không package nào khác đụng tới.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS users (id UUID PRIMARY KEY)`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS email VARCHAR(255)`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS full_name VARCHAR(255)`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255)`,

	`CREATE TABLE IF NOT EXISTS projects (id UUID PRIMARY KEY)`,
	`ALTER TABLE projects ADD COLUMN IF NOT EXISTS name VARCHAR(255)`,
	`ALTER TABLE projects ADD COLUMN IF NOT EXISTS code VARCHAR(50)`,
	`ALTER TABLE projects ADD COLUMN IF NOT EXISTS ragflow_dataset_id VARCHAR(64)`,
	`ALTER TABLE projects ADD COLUMN IF NOT EXISTS ragflow_chat_id VARCHAR(64)`,
	`ALTER TABLE projects ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ`,
	`ALTER TABLE projects ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ`,

	`CREATE TABLE IF NOT EXISTS project_members (project_id UUID)`,
	`ALTER TABLE project_members ADD COLUMN IF NOT EXISTS id UUID`,
	`ALTER TABLE project_members ADD COLUMN IF NOT EXISTS user_id UUID`,
	`ALTER TABLE project_members ADD COLUMN IF NOT EXISTS role VARCHAR(20)`,

	`CREATE TABLE IF NOT EXISTS project_reports (
		id UUID PRIMARY KEY, project_id UUID NOT NULL,
		report_type VARCHAR(20) NOT NULL CHECK (report_type IN ('uat','planning','testcase')),
		format VARCHAR(10) NOT NULL CHECK (format IN ('xlsx','pdf')),
		file_path VARCHAR(500) NOT NULL, generated_by UUID NOT NULL,
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
	// CHỈ dọn 2 bảng CỦA RIÊNG module report — không TRUNCATE users/projects/
	// project_members vì đó là bảng dùng chung với package khác (xem comment
	// ở schemaStatements); mỗi test đã tự cô lập bằng uuid.New() ngẫu nhiên
	// nên không cần dọn bảng dùng chung để đảm bảo cô lập.
	require.NoError(t, db.Exec(`TRUNCATE TABLE project_report_items, project_reports`).Error)
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

// seedProject chèn user/project mới với UUID ngẫu nhiên — cô lập giữa các
// test mà không cần TRUNCATE bảng dùng chung. Cung cấp đủ full_name/
// password_hash phòng khi bảng users đã tồn tại với schema đầy đủ NOT NULL
// (do package auth/user tạo trước) lẫn khi report tự tạo bảng trước (cột
// nullable qua ALTER) — cả hai trường hợp INSERT đều hợp lệ.
func seedProject(t *testing.T, db *gorm.DB, projectID, userID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, full_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID.String()+"@example.com", "Integration Test", "hash").Error)
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
	// id chèn tường minh vì cột id (nếu package khác đã tạo bảng project_members
	// trước) có thể là PRIMARY KEY NOT NULL không default — xem schemaStatements.
	require.NoError(t, db.Exec(
		`INSERT INTO project_members (id, project_id, user_id, role) VALUES (?, ?, ?, ?)`,
		uuid.New(), projectID, userID, "editor").Error)

	role, err := repo.MemberRole(ctx, projectID, userID)
	require.NoError(t, err)
	require.Equal(t, "editor", role)

	name, code, err := repo.ProjectMeta(ctx, projectID)
	require.NoError(t, err)
	require.Equal(t, "Demo Project", name)
	require.Equal(t, "DEMO", code)
}
