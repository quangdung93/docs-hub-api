// Package repository đọc mapping project/version/revision sang RAGFlow IDs.
package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error) {
	var role string
	err := postgres.DBFrom(ctx, r.db).Table("project_members").Select("role").
		Where("project_id=? AND user_id=?", projectID, actorID).Scan(&role).Error
	return role, err
}

func (r *Repository) ScopeExists(ctx context.Context, projectID uuid.UUID, scope domain.Scope) (bool, error) {
	var count int64
	query := postgres.DBFrom(ctx, r.db)
	if scope.VersionID != nil {
		query = query.Table("project_versions").Where("project_id=? AND id=?", projectID, *scope.VersionID)
	} else {
		query = query.Table("change_requests").Where("project_id=? AND id=?", projectID, *scope.ChangeRequestID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) DatasetID(ctx context.Context, projectID uuid.UUID) (string, error) {
	var datasetID string
	err := postgres.DBFrom(ctx, r.db).Table("projects").Select("ragflow_dataset_id").
		Where("id=? AND deleted_at IS NULL", projectID).Scan(&datasetID).Error
	return datasetID, err
}

func (r *Repository) RevisionRefs(ctx context.Context, projectID uuid.UUID, scope domain.Scope) ([]domain.RevisionRef, error) {
	query := postgres.DBFrom(ctx, r.db).Table("document_revisions r").
		Select(`r.document_id,r.id AS revision_id,d.title,r.ragflow_document_id`).
		Joins("JOIN documents d ON d.id=r.document_id AND d.deleted_at IS NULL").
		Where(`r.project_id=? AND r.status='ready' AND r.ragflow_sync_status='ready'
			AND r.ragflow_document_id IS NOT NULL`, projectID)
	if scope.VersionID != nil {
		query = query.Where("r.project_version_id=?", *scope.VersionID)
	} else {
		query = query.Where("r.change_request_id=?", *scope.ChangeRequestID)
	}
	var rows []struct {
		DocumentID        uuid.UUID `gorm:"column:document_id"`
		RevisionID        uuid.UUID `gorm:"column:revision_id"`
		Title             string    `gorm:"column:title"`
		RAGFlowDocumentID string    `gorm:"column:ragflow_document_id"`
	}
	if err := query.Order("r.document_id,r.revision_no DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.RevisionRef, len(rows))
	for i, row := range rows {
		out[i] = domain.RevisionRef{
			DocumentID: row.DocumentID, RevisionID: row.RevisionID,
			Title: row.Title, RAGFlowDocumentID: row.RAGFlowDocumentID,
		}
	}
	return out, nil
}
