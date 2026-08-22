package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type projectSeed struct {
	id, code, name, description string
	versions                    []versionSeed
	changeRequests              []changeRequestSeed
}

type versionSeed struct {
	id, label, status string
	sequence          int
	releasedAt        any
}

type changeRequestSeed struct {
	id, code, title, status string
	sequence                int
}

var projectSeeds = []projectSeed{ //nolint:gochecknoglobals // fixture seed bất biến
	{
		id: "10000000-0000-4000-8000-000000000001", code: "DOCS-HUB-DEMO",
		name: "Docs Hub Demo", description: "Project mẫu để thử upload và ingestion tài liệu",
		versions: []versionSeed{
			{id: "11000000-0000-4000-8000-000000000001", label: "v1.0.0", status: "published", sequence: 1, releasedAt: "2026-01-15T00:00:00Z"},
			{id: "11000000-0000-4000-8000-000000000002", label: "v1.1.0", status: "draft", sequence: 2},
		},
		changeRequests: []changeRequestSeed{
			{id: "12000000-0000-4000-8000-000000000001", code: "CR-001", title: "Bổ sung tìm kiếm tài liệu", status: "accepted", sequence: 1},
			{id: "12000000-0000-4000-8000-000000000002", code: "CR-002", title: "Hỗ trợ tài liệu Office", status: "review", sequence: 2},
			{id: "12000000-0000-4000-8000-000000000003", code: "CR-003", title: "Cải thiện citation", status: "draft", sequence: 3},
		},
	},
	{
		id: "20000000-0000-4000-8000-000000000001", code: "API-PORTAL-DEMO",
		name: "API Portal Demo", description: "Project mẫu thứ hai để kiểm tra cô lập dữ liệu",
		versions: []versionSeed{
			{id: "21000000-0000-4000-8000-000000000001", label: "2026.1", status: "published", sequence: 1, releasedAt: "2026-02-01T00:00:00Z"},
			{id: "21000000-0000-4000-8000-000000000002", label: "2026.2", status: "draft", sequence: 2},
		},
		changeRequests: []changeRequestSeed{
			{id: "22000000-0000-4000-8000-000000000001", code: "CR-API-001", title: "Thêm API quản lý token", status: "review", sequence: 1},
			{id: "22000000-0000-4000-8000-000000000002", code: "CR-API-002", title: "Chuẩn hóa error response", status: "draft", sequence: 2},
		},
	},
}

func seedProjects(ctx context.Context, db *gorm.DB, adminID uuid.UUID) error {
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, project := range projectSeeds {
			if err := upsertProject(tx, project, adminID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("seed project fixtures: %w", err)
	}
	printProjectSeeds()
	return nil
}

func upsertProject(tx *gorm.DB, project projectSeed, adminID uuid.UUID) error {
	const projectSQL = `INSERT INTO projects(id,code,name,description,status,owner_id,version)
		VALUES(?,?,?,?, 'active', ?,1)
		ON CONFLICT(id) DO UPDATE SET code=excluded.code,name=excluded.name,
		description=excluded.description,status='active',owner_id=excluded.owner_id,
		deleted_at=NULL,updated_at=now()`
	if err := tx.Exec(projectSQL, project.id, project.code, project.name, project.description, adminID).Error; err != nil {
		return fmt.Errorf("upsert project %s: %w", project.code, err)
	}
	// Chốt theo id (giống projectSQL ở trên) chứ KHÔNG theo (project_id,user_id).
	// Bản ghi seed dùng chính id dự án làm id thành viên, nên khi ĐỔI tài khoản
	// admin thì user_id khác đi và mệnh đề (project_id,user_id) không khớp —
	// Postgres cố INSERT thật rồi đụng khoá chính. Chốt theo id vừa idempotent
	// vừa cập nhật được chủ sở hữu mới.
	const memberSQL = `INSERT INTO project_members(id,project_id,user_id,role,status,joined_at)
		VALUES(?,?,?,'owner','active',now()) ON CONFLICT(id)
		DO UPDATE SET user_id=excluded.user_id,role='owner',status='active',
		joined_at=COALESCE(project_members.joined_at,now())`
	if err := tx.Exec(memberSQL, project.id, project.id, adminID).Error; err != nil {
		return fmt.Errorf("upsert owner project %s: %w", project.code, err)
	}
	for _, version := range project.versions {
		const versionSQL = `INSERT INTO project_versions(
			id,project_id,label,sequence_no,status,released_at,created_by)
			VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET label=excluded.label,
			sequence_no=excluded.sequence_no,status=excluded.status,released_at=excluded.released_at,
			created_by=excluded.created_by,updated_at=now()`
		if err := tx.Exec(versionSQL, version.id, project.id, version.label, version.sequence,
			version.status, version.releasedAt, adminID).Error; err != nil {
			return fmt.Errorf("upsert version %s/%s: %w", project.code, version.label, err)
		}
	}
	for _, change := range project.changeRequests {
		const changeSQL = `INSERT INTO change_requests(
			id,project_id,code,title,status,sequence_no,created_by)
			VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET code=excluded.code,
			title=excluded.title,status=excluded.status,sequence_no=excluded.sequence_no,
			created_by=excluded.created_by,updated_at=now()`
		if err := tx.Exec(changeSQL, change.id, project.id, change.code, change.title,
			change.status, change.sequence, adminID).Error; err != nil {
			return fmt.Errorf("upsert change request %s/%s: %w", project.code, change.code, err)
		}
	}
	return nil
}

func printProjectSeeds() {
	fmt.Println("seed: project/version/change-request mẫu đã sẵn sàng")
	for _, project := range projectSeeds {
		fmt.Printf("  project %s: %s\n", project.code, project.id)
		for _, version := range project.versions {
			fmt.Printf("    version %-10s (%s): %s\n", version.label, version.status, version.id)
		}
		for _, change := range project.changeRequests {
			fmt.Printf("    change  %-10s (%s): %s\n", change.code, change.status, change.id)
		}
	}
}
