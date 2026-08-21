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

type projectVersionRepository struct{ db *gorm.DB }

func NewProjectVersionRepository(db *gorm.DB) domain.ProjectVersionRepository {
	return &projectVersionRepository{db: db}
}

type projectVersionModel struct {
	ID, ProjectID, Label, Status, CreatedBy string
	SequenceNo                              int64
	ReleasedAt                              *time.Time
	CreatedAt, UpdatedAt                    time.Time
}

func (projectVersionModel) TableName() string { return "project_versions" }

func (r *projectVersionRepository) Create(ctx context.Context, v *domain.ProjectVersion, requestID string) error {
	db := postgres.DBFrom(ctx, r.db)
	var project struct{ ID string }
	if err := db.Raw(`SELECT id FROM projects WHERE id=? FOR UPDATE`, v.ProjectID).Scan(&project).Error; err != nil {
		return fmt.Errorf("khóa project: %w", err)
	}
	if project.ID == "" {
		return domain.ErrNotFound
	}
	if err := db.Raw(`SELECT COALESCE(MAX(sequence_no),0)+1 AS sequence_no FROM project_versions WHERE project_id=?`, v.ProjectID).
		Scan(&v.SequenceNo).Error; err != nil {
		return fmt.Errorf("cấp sequence version: %w", err)
	}
	m := projectVersionModel{
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
		"request_id": requestID, "metadata": "{}", "created_at": v.CreatedAt,
	}
	if err := db.Table("audit_logs").Create(audit).Error; err != nil {
		return fmt.Errorf("ghi audit tạo version: %w", postgres.Translate(err))
	}
	return nil
}

func (r *projectVersionRepository) List(
	ctx context.Context, projectID uuid.UUID, page pagination.Query,
) ([]domain.ProjectVersion, int64, error) {
	page = page.Normalize()
	base := postgres.DBFrom(ctx, r.db).Model(&projectVersionModel{}).Where("project_id=?", projectID)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("đếm project version: %w", postgres.Translate(err))
	}
	var models []projectVersionModel
	if err := base.Order("sequence_no DESC,id DESC").Limit(page.Limit).Offset(page.Offset()).Find(&models).Error; err != nil {
		return nil, 0, fmt.Errorf("liệt kê project version: %w", postgres.Translate(err))
	}
	versions := make([]domain.ProjectVersion, len(models))
	for i := range models {
		id, err := uuid.Parse(models[i].ID)
		if err != nil {
			return nil, 0, fmt.Errorf("parse version id: %w", err)
		}
		createdBy, err := uuid.Parse(models[i].CreatedBy)
		if err != nil {
			return nil, 0, fmt.Errorf("parse version created_by: %w", err)
		}
		versions[i] = domain.ProjectVersion{
			ID: id, ProjectID: projectID, Label: models[i].Label, SequenceNo: models[i].SequenceNo,
			Status: models[i].Status, ReleasedAt: models[i].ReleasedAt, CreatedBy: createdBy,
			CreatedAt: models[i].CreatedAt, UpdatedAt: models[i].UpdatedAt,
		}
	}
	return versions, total, nil
}
