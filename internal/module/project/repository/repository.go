// Package repository hiện thực project persistence bằng PostgreSQL/GORM.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

type projectModel struct {
	ID, Code, Name, Description, Status, OwnerID string
	Version                                      int
	RAGFlowDatasetID                             string `gorm:"column:ragflow_dataset_id"`
	RAGFlowSyncStatus                            string `gorm:"column:ragflow_sync_status"`
	RAGFlowLastError                             string `gorm:"column:ragflow_last_error"`
	CreatedAt, UpdatedAt                         time.Time
}

func (projectModel) TableName() string { return "projects" }

type versionModel struct {
	ID, ProjectID, Label, Status, CreatedBy string
	SequenceNo                              int64
	ReleasedAt                              *time.Time
	CreatedAt, UpdatedAt                    time.Time
}

func (versionModel) TableName() string { return "project_versions" }

func (r *Repository) CodeExists(ctx context.Context, code string) (bool, error) {
	var count int64
	err := postgres.DBFrom(ctx, r.db).Model(&projectModel{}).
		Where("lower(code)=lower(?) AND deleted_at IS NULL", code).Count(&count).Error
	return count > 0, translateProjectError(err)
}

func (r *Repository) Create(ctx context.Context, params domain.CreateParams) error {
	p := params.Project
	m := projectModel{
		ID: p.ID.String(), Code: p.Code, Name: p.Name, Description: p.Description,
		Status: p.Status, OwnerID: p.OwnerID.String(), Version: p.Version,
		RAGFlowDatasetID: p.RAGFlowDatasetID, RAGFlowSyncStatus: p.RAGFlowSyncStatus,
		RAGFlowLastError: p.RAGFlowLastError, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	db := postgres.DBFrom(ctx, r.db)
	if err := db.Create(&m).Error; err != nil {
		return translateProjectError(err)
	}
	membership := map[string]any{
		"project_id": p.ID, "user_id": p.OwnerID, "role": domain.RoleOwner,
		"created_at": p.CreatedAt, "updated_at": p.UpdatedAt,
	}
	if err := db.Table("project_members").Create(membership).Error; err != nil {
		return fmt.Errorf("tạo owner membership: %w", translateProjectError(err))
	}
	audit := map[string]any{
		"id": uuid.New(), "actor_user_id": p.OwnerID, "project_id": p.ID,
		"action": "project.created", "entity_type": "project", "entity_id": p.ID,
		"request_id": params.RequestID, "metadata": "{}", "created_at": p.CreatedAt,
	}
	if err := db.Table("audit_logs").Create(audit).Error; err != nil {
		return fmt.Errorf("ghi audit tạo project: %w", translateProjectError(err))
	}
	return nil
}

func (r *Repository) MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error) {
	var role string
	err := postgres.DBFrom(ctx, r.db).Table("project_members").Select("role").
		Where("project_id=? AND user_id=?", projectID, actorID).Scan(&role).Error
	return role, translateProjectError(err)
}

// CreateVersion khóa row project để hai request song song không nhận cùng
// sequence_no. Unique label trong PostgreSQL vẫn là lớp bảo vệ cuối.
func (r *Repository) CreateVersion(ctx context.Context, params domain.CreateVersionParams) error {
	db := postgres.DBFrom(ctx, r.db)
	v := params.Version
	var project struct{ ID string }
	if err := db.Raw(`SELECT id FROM projects WHERE id=? AND deleted_at IS NULL FOR UPDATE`, v.ProjectID).
		Scan(&project).Error; err != nil {
		return fmt.Errorf("khóa project: %w", err)
	}
	if project.ID == "" {
		return domain.ErrProjectNotFound
	}
	if err := db.Raw(`SELECT COALESCE(MAX(sequence_no),0)+1 AS sequence_no FROM project_versions WHERE project_id=?`, v.ProjectID).
		Scan(&v.SequenceNo).Error; err != nil {
		return fmt.Errorf("cấp sequence version: %w", err)
	}
	m := versionModel{
		ID: v.ID.String(), ProjectID: v.ProjectID.String(), Label: v.Label,
		SequenceNo: v.SequenceNo, Status: v.Status, ReleasedAt: v.ReleasedAt,
		CreatedBy: v.CreatedBy.String(), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
	if err := db.Create(&m).Error; err != nil {
		if errors.Is(postgres.Translate(err), postgres.ErrDuplicateKey) {
			return domain.ErrDuplicateVersionLabel
		}
		return fmt.Errorf("tạo project version: %w", postgres.Translate(err))
	}
	audit := map[string]any{
		"id": uuid.New(), "actor_user_id": v.CreatedBy, "project_id": v.ProjectID,
		"action": "project_version.created", "entity_type": "project_version", "entity_id": v.ID,
		"request_id": params.RequestID, "metadata": "{}", "created_at": v.CreatedAt,
	}
	if err := db.Table("audit_logs").Create(audit).Error; err != nil {
		return fmt.Errorf("ghi audit tạo version: %w", postgres.Translate(err))
	}
	return nil
}

func (r *Repository) ListVersions(
	ctx context.Context, projectID uuid.UUID, page pagination.Query,
) ([]domain.ProjectVersion, int64, error) {
	page = page.Normalize()
	base := postgres.DBFrom(ctx, r.db).Model(&versionModel{}).Where("project_id=?", projectID)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("đếm project version: %w", postgres.Translate(err))
	}
	var models []versionModel
	if err := base.Order("sequence_no DESC,id DESC").Limit(page.Limit).Offset(page.Offset()).Find(&models).Error; err != nil {
		return nil, 0, fmt.Errorf("liệt kê project version: %w", postgres.Translate(err))
	}
	versions := make([]domain.ProjectVersion, len(models))
	for i := range models {
		version, err := toVersion(models[i])
		if err != nil {
			return nil, 0, err
		}
		versions[i] = version
	}
	return versions, total, nil
}

func toVersion(model versionModel) (domain.ProjectVersion, error) {
	id, err := uuid.Parse(model.ID)
	if err != nil {
		return domain.ProjectVersion{}, fmt.Errorf("parse version id: %w", err)
	}
	projectID, err := uuid.Parse(model.ProjectID)
	if err != nil {
		return domain.ProjectVersion{}, fmt.Errorf("parse version project id: %w", err)
	}
	createdBy, err := uuid.Parse(model.CreatedBy)
	if err != nil {
		return domain.ProjectVersion{}, fmt.Errorf("parse version created_by: %w", err)
	}
	return domain.ProjectVersion{
		ID: id, ProjectID: projectID, Label: model.Label, SequenceNo: model.SequenceNo,
		Status: model.Status, ReleasedAt: model.ReleasedAt, CreatedBy: createdBy,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}, nil
}

func translateProjectError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(postgres.Translate(err), postgres.ErrDuplicateKey) {
		return domain.ErrDuplicateCode
	}
	return fmt.Errorf("project repository: %w", postgres.Translate(err))
}
