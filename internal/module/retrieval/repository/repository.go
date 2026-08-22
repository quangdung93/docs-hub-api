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
		Where("project_id=? AND user_id=? AND status='active'", projectID, actorID).Scan(&role).Error
	return role, err
}

func (r *Repository) ResolveScope(
	ctx context.Context, projectID uuid.UUID, scope domain.Scope,
) ([]domain.ResolvedScope, error) {
	if scope.Mode == domain.ScopeAll {
		return nil, nil
	}
	rows := make([]struct {
		ID, Label string
	}, 0)
	query := postgres.DBFrom(ctx, r.db)
	scopeType := "version"
	if scope.Mode == domain.ScopeVersions {
		query = query.Table("project_versions").Select("id,label").
			Where("project_id=? AND id IN ?", projectID, scope.VersionIDs).
			Order("sequence_no,id")
	} else {
		scopeType = "change_request"
		query = query.Table("change_requests").Select("id,code AS label").
			Where("project_id=? AND id IN ?", projectID, scope.ChangeRequestIDs).
			Order("sequence_no,id")
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	resolved := make([]domain.ResolvedScope, 0, len(rows))
	for _, row := range rows {
		id, err := uuid.Parse(row.ID)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, domain.ResolvedScope{ID: id, Type: scopeType, Label: row.Label})
	}
	return resolved, nil
}

func (r *Repository) DatasetID(ctx context.Context, projectID uuid.UUID) (string, error) {
	var datasetID string
	err := postgres.DBFrom(ctx, r.db).Table("projects").Select("ragflow_dataset_id").
		Where("id=? AND deleted_at IS NULL", projectID).Scan(&datasetID).Error
	return datasetID, err
}

func (r *Repository) RevisionRefs(ctx context.Context, projectID uuid.UUID, scope domain.Scope) ([]domain.RevisionRef, error) {
	query := postgres.DBFrom(ctx, r.db).Table("document_revisions r").
		Select(`r.document_id,r.id AS revision_id,d.title,r.file_name,r.ragflow_document_id,
			r.project_version_id,r.change_request_id,pv.label AS version_label,cr.code AS change_label`).
		Joins("JOIN documents d ON d.id=r.document_id AND d.deleted_at IS NULL").
		Joins("LEFT JOIN project_versions pv ON pv.id=r.project_version_id AND pv.project_id=r.project_id").
		Joins("LEFT JOIN change_requests cr ON cr.id=r.change_request_id AND cr.project_id=r.project_id").
		Where(`r.project_id=? AND r.status='ready' AND r.ragflow_sync_status='ready'
			AND r.ragflow_document_id IS NOT NULL`, projectID)
	if scope.Mode == domain.ScopeVersions {
		query = query.Where("r.project_version_id IN ?", scope.VersionIDs)
	} else if scope.Mode == domain.ScopeChangeRequests {
		query = query.Where("r.change_request_id IN ?", scope.ChangeRequestIDs)
	}
	var rows []struct {
		DocumentID        uuid.UUID  `gorm:"column:document_id"`
		RevisionID        uuid.UUID  `gorm:"column:revision_id"`
		Title             string     `gorm:"column:title"`
		FileName          string     `gorm:"column:file_name"`
		RAGFlowDocumentID string     `gorm:"column:ragflow_document_id"`
		ProjectVersionID  *uuid.UUID `gorm:"column:project_version_id"`
		ChangeRequestID   *uuid.UUID `gorm:"column:change_request_id"`
		VersionLabel      string     `gorm:"column:version_label"`
		ChangeLabel       string     `gorm:"column:change_label"`
	}
	if err := query.Order(`COALESCE(pv.sequence_no,cr.sequence_no),
		CASE WHEN r.project_version_id IS NOT NULL THEN 0 ELSE 1 END,r.document_id,r.revision_no DESC`).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.RevisionRef, len(rows))
	for i, row := range rows {
		resolved := domain.ResolvedScope{Type: "version", Label: row.VersionLabel}
		if row.ProjectVersionID != nil {
			resolved.ID = *row.ProjectVersionID
		} else if row.ChangeRequestID != nil {
			resolved = domain.ResolvedScope{ID: *row.ChangeRequestID, Type: "change_request", Label: row.ChangeLabel}
		}
		out[i] = domain.RevisionRef{
			DocumentID: row.DocumentID, RevisionID: row.RevisionID,
			Title: row.Title, FileName: row.FileName, Scope: resolved,
			RAGFlowDocumentID: row.RAGFlowDocumentID,
		}
	}
	return out, nil
}
