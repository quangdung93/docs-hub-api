// Package repository lưu metadata tài liệu bằng PostgreSQL/GORM.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

type documentModel struct {
	ID, ProjectID, Title, DocumentKey, Description, SourceType, CreatedBy string
	Version                                                               int
	CreatedAt, UpdatedAt                                                  time.Time
	DeletedAt                                                             gorm.DeletedAt
}

func (documentModel) TableName() string { return "documents" }

type revisionModel struct {
	ID, DocumentID, ProjectID         string
	ProjectVersionID, ChangeRequestID *string
	RevisionNo                        int
	FileName, MediaType               string
	SizeBytes                         int64
	SHA256, ObjectKey, Status         string
	ErrorCode, ErrorDetail            string `gorm:"column:error_detail_sanitized"`
	CreatedBy                         string
	CreatedAt, UpdatedAt              time.Time
}

func (revisionModel) TableName() string { return "document_revisions" }

type uploadModel struct {
	ID, ProjectID, DocumentID, RevisionID   string
	ProjectVersionID, ChangeRequestID       *string
	Title, Description, FileName, MediaType string
	SizeBytes                               int64
	SHA256, ObjectKey, Status, CreatedBy    string
	ExpiresAt, CreatedAt                    time.Time
	CompletedAt                             *time.Time
}

func (uploadModel) TableName() string { return "document_uploads" }

func (r *Repository) MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error) {
	var role string
	err := postgres.DBFrom(ctx, r.db).Table("project_members").Select("role").
		Where("project_id=? AND user_id=?", projectID, actorID).Scan(&role).Error
	return role, err
}
func (r *Repository) ScopeExists(ctx context.Context, projectID uuid.UUID, s domain.Scope) (bool, error) {
	var count int64
	q := postgres.DBFrom(ctx, r.db)
	if s.VersionID != nil {
		q = q.Table("project_versions").Where("project_id=? AND id=?", projectID, *s.VersionID)
	} else {
		q = q.Table("change_requests").Where("project_id=? AND id=?", projectID, *s.ChangeRequestID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
func (r *Repository) CreateRevision(ctx context.Context, in domain.CreateRevisionParams) (*domain.Document, *domain.Revision, error) {
	db := postgres.DBFrom(ctx, r.db)
	var d documentModel
	err := db.First(&d, "id=? AND project_id=?", in.DocumentID, in.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		d = documentModel{
			ID: in.DocumentID.String(), ProjectID: in.ProjectID.String(), Title: in.Title,
			DocumentKey: in.DocumentID.String(), Description: in.Description,
			SourceType: "upload", CreatedBy: in.ActorID.String(), Version: 1,
		}
		if err := db.Create(&d).Error; err != nil {
			return nil, nil, fmt.Errorf("tạo document: %w", err)
		}
	} else if err != nil {
		return nil, nil, mapErr(err)
	}
	var n int64
	if err := db.Model(&revisionModel{}).Where("document_id=?", d.ID).Count(&n).Error; err != nil {
		return nil, nil, err
	}
	m := revisionModel{
		ID: in.RevisionID.String(), DocumentID: d.ID, ProjectID: in.ProjectID.String(),
		RevisionNo: int(n) + 1, FileName: in.FileName, MediaType: in.MediaType,
		SizeBytes: in.SizeBytes, SHA256: in.SHA256, ObjectKey: in.ObjectKey,
		Status: "queued", CreatedBy: in.ActorID.String(),
	}
	setScope(&m, in.Scope)
	if err := db.Create(&m).Error; err != nil {
		return nil, nil, fmt.Errorf("tạo revision: %w", err)
	}
	if err := r.enqueue(ctx, &m, in.ActorID, "document.uploaded"); err != nil {
		return nil, nil, err
	}
	return toDocument(d), toRevision(m), nil
}
func (r *Repository) enqueue(ctx context.Context, m *revisionModel, actor uuid.UUID, action string) error {
	db := postgres.DBFrom(ctx, r.db)
	jobID, eventID, auditID := uuid.New(), uuid.New(), uuid.New()
	payload, _ := json.Marshal(map[string]string{"revision_id": m.ID, "project_id": m.ProjectID})
	job := map[string]any{"id": jobID, "document_revision_id": m.ID, "status": "pending"}
	if err := db.Table("ingestion_jobs").Create(job).Error; err != nil {
		return fmt.Errorf("tạo ingestion job: %w", err)
	}
	event := map[string]any{
		"id": eventID, "topic": "document.ingest", "aggregate_type": "document_revision",
		"aggregate_id": m.ID, "payload": string(payload), "status": "pending",
	}
	if err := db.Table("outbox_events").Create(event).Error; err != nil {
		return fmt.Errorf("tạo outbox: %w", err)
	}
	audit := map[string]any{
		"id": auditID, "actor_user_id": actor, "project_id": m.ProjectID,
		"action": action, "entity_type": "document_revision", "entity_id": m.ID, "metadata": "{}",
	}
	return db.Table("audit_logs").Create(audit).Error
}
func (r *Repository) CreateUpload(ctx context.Context, u *domain.Upload) error {
	return postgres.DBFrom(ctx, r.db).Create(fromUpload(u)).Error
}
func (r *Repository) FindUpload(ctx context.Context, pid, uid uuid.UUID) (*domain.Upload, error) {
	var m uploadModel
	if err := postgres.DBFrom(ctx, r.db).First(&m, "id=? AND project_id=?", uid, pid).Error; err != nil {
		return nil, mapErr(err)
	}
	return toUpload(m), nil
}
func (r *Repository) CompleteUpload(ctx context.Context, u *domain.Upload) (*domain.Document, *domain.Revision, error) {
	db := postgres.DBFrom(ctx, r.db)
	claim := db.Model(&uploadModel{}).Where("id=? AND project_id=? AND status='pending'", u.ID, u.ProjectID).
		Update("status", "completing")
	if claim.Error != nil {
		return nil, nil, claim.Error
	}
	if claim.RowsAffected == 0 {
		return nil, nil, domain.ErrConflict
	}
	params := domain.CreateRevisionParams{
		DocumentID: u.DocumentID, RevisionID: u.RevisionID, ProjectID: u.ProjectID,
		ActorID: u.CreatedBy, Scope: u.Scope, Title: u.Title, Description: u.Description,
		FileName: u.FileName, MediaType: u.MediaType, SHA256: u.SHA256,
		ObjectKey: u.ObjectKey, SizeBytes: u.SizeBytes,
	}
	d, rev, err := r.CreateRevision(ctx, params)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	updates := map[string]any{"status": "completed", "completed_at": now}
	if err := db.Model(&uploadModel{}).
		Where("id=? AND status='completing'", u.ID).Updates(updates).Error; err != nil {
		return nil, nil, err
	}
	return d, rev, nil
}
func (r *Repository) List(ctx context.Context, pid uuid.UUID, f domain.Filter, p pagination.Query) ([]domain.Document, int64, error) {
	p = p.Normalize()
	q := postgres.DBFrom(ctx, r.db).Model(&documentModel{}).Where("project_id=?", pid)
	if f.Query != "" {
		q = q.Where("title ILIKE ?", "%"+f.Query+"%")
	}
	if f.Status != "" || f.MediaType != "" || f.VersionID != nil || f.ChangeRequestID != nil {
		const revisionFilter = `EXISTS (SELECT 1 FROM document_revisions r
			WHERE r.document_id=documents.id AND (?='' OR r.status=?)
			AND (?='' OR r.media_type=?) AND (? IS NULL OR r.project_version_id=?)
			AND (? IS NULL OR r.change_request_id=?))`
		q = q.Where(revisionFilter, f.Status, f.Status, f.MediaType, f.MediaType,
			f.VersionID, f.VersionID, f.ChangeRequestID, f.ChangeRequestID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ms []documentModel
	if err := q.Order("updated_at DESC,id DESC").Limit(p.Limit).Offset(p.Offset()).Find(&ms).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Document, len(ms))
	for i, m := range ms {
		out[i] = *toDocument(m)
	}
	return out, total, nil
}
func (r *Repository) FindDocument(ctx context.Context, pid, did uuid.UUID) (*domain.Document, []domain.Revision, error) {
	var d documentModel
	if err := postgres.DBFrom(ctx, r.db).First(&d, "id=? AND project_id=?", did, pid).Error; err != nil {
		return nil, nil, mapErr(err)
	}
	var ms []revisionModel
	if err := postgres.DBFrom(ctx, r.db).Where("document_id=?", did).Order("revision_no DESC").Find(&ms).Error; err != nil {
		return nil, nil, err
	}
	rs := make([]domain.Revision, len(ms))
	for i, m := range ms {
		rs[i] = *toRevision(m)
	}
	return toDocument(d), rs, nil
}
func (r *Repository) FindRevision(ctx context.Context, pid, did, rid uuid.UUID) (*domain.Revision, error) {
	var m revisionModel
	if err := postgres.DBFrom(ctx, r.db).First(&m, "id=? AND document_id=? AND project_id=?", rid, did, pid).Error; err != nil {
		return nil, mapErr(err)
	}
	return toRevision(m), nil
}
func (r *Repository) Update(ctx context.Context, pid, did uuid.UUID, title, desc string, v int) (*domain.Document, error) {
	updates := map[string]any{
		"title": title, "description": desc,
		"version": gorm.Expr("version+1"), "updated_at": time.Now().UTC(),
	}
	res := postgres.DBFrom(ctx, r.db).Model(&documentModel{}).
		Where("id=? AND project_id=? AND version=?", did, pid, v).Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrConflict
	}
	d, _, err := r.FindDocument(ctx, pid, did)
	return d, err
}
func (r *Repository) Retry(ctx context.Context, pid, did, rid, actor uuid.UUID) error {
	db := postgres.DBFrom(ctx, r.db)
	var m revisionModel
	if err := db.First(&m, "id=? AND document_id=? AND project_id=?", rid, did, pid).Error; err != nil {
		return mapErr(err)
	}
	if m.Status != "failed" {
		return domain.ErrConflict
	}
	if err := db.Model(&m).Updates(map[string]any{"status": "queued", "error_code": nil, "error_detail_sanitized": nil}).Error; err != nil {
		return err
	}
	return r.enqueue(ctx, &m, actor, "document.retry")
}
func (r *Repository) SoftDelete(ctx context.Context, pid, did, actor uuid.UUID) error {
	res := postgres.DBFrom(ctx, r.db).Where("id=? AND project_id=?", did, pid).Delete(&documentModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	payload, _ := json.Marshal(map[string]string{"document_id": did.String(), "project_id": pid.String()})
	if err := postgres.DBFrom(ctx, r.db).Table("outbox_events").Create(map[string]any{
		"id": uuid.New(), "topic": "document.cleanup", "aggregate_type": "document",
		"aggregate_id": did, "payload": string(payload), "status": "pending",
	}).Error; err != nil {
		return fmt.Errorf("tạo cleanup event: %w", err)
	}
	audit := map[string]any{
		"id": uuid.New(), "actor_user_id": actor, "project_id": pid,
		"action": "document.deleted", "entity_type": "document", "entity_id": did, "metadata": "{}",
	}
	return postgres.DBFrom(ctx, r.db).Table("audit_logs").Create(audit).Error
}
func setScope(m *revisionModel, s domain.Scope) {
	if s.VersionID != nil {
		x := s.VersionID.String()
		m.ProjectVersionID = &x
	} else {
		x := s.ChangeRequestID.String()
		m.ChangeRequestID = &x
	}
}
func ptrID(s *string) *uuid.UUID {
	if s == nil {
		return nil
	}
	x := uuid.MustParse(*s)
	return &x
}
func toDocument(m documentModel) *domain.Document {
	return &domain.Document{
		ID: uuid.MustParse(m.ID), ProjectID: uuid.MustParse(m.ProjectID),
		CreatedBy: uuid.MustParse(m.CreatedBy), Title: m.Title, Key: m.DocumentKey,
		Description: m.Description, Version: m.Version, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}
func toRevision(m revisionModel) *domain.Revision {
	return &domain.Revision{
		ID: uuid.MustParse(m.ID), DocumentID: uuid.MustParse(m.DocumentID),
		ProjectID: uuid.MustParse(m.ProjectID), CreatedBy: uuid.MustParse(m.CreatedBy),
		Scope:      domain.Scope{VersionID: ptrID(m.ProjectVersionID), ChangeRequestID: ptrID(m.ChangeRequestID)},
		RevisionNo: m.RevisionNo, FileName: m.FileName, MediaType: m.MediaType,
		SizeBytes: m.SizeBytes, SHA256: m.SHA256, ObjectKey: m.ObjectKey, Status: m.Status,
		ErrorCode: m.ErrorCode, ErrorDetail: m.ErrorDetail, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}
func fromUpload(u *domain.Upload) *uploadModel {
	m := &uploadModel{
		ID: u.ID.String(), ProjectID: u.ProjectID.String(), DocumentID: u.DocumentID.String(),
		RevisionID: u.RevisionID.String(), Title: u.Title, Description: u.Description,
		FileName: u.FileName, MediaType: u.MediaType, SizeBytes: u.SizeBytes,
		SHA256: u.SHA256, ObjectKey: u.ObjectKey, Status: u.Status,
		CreatedBy: u.CreatedBy.String(), ExpiresAt: u.ExpiresAt,
	}
	if u.Scope.VersionID != nil {
		x := u.Scope.VersionID.String()
		m.ProjectVersionID = &x
	} else {
		x := u.Scope.ChangeRequestID.String()
		m.ChangeRequestID = &x
	}
	return m
}
func toUpload(m uploadModel) *domain.Upload {
	return &domain.Upload{
		ID: uuid.MustParse(m.ID), ProjectID: uuid.MustParse(m.ProjectID),
		DocumentID: uuid.MustParse(m.DocumentID), RevisionID: uuid.MustParse(m.RevisionID),
		CreatedBy: uuid.MustParse(m.CreatedBy),
		Scope:     domain.Scope{VersionID: ptrID(m.ProjectVersionID), ChangeRequestID: ptrID(m.ChangeRequestID)},
		Title:     m.Title, Description: m.Description, FileName: m.FileName, MediaType: m.MediaType,
		SizeBytes: m.SizeBytes, SHA256: m.SHA256, ObjectKey: m.ObjectKey,
		Status: m.Status, ExpiresAt: m.ExpiresAt,
	}
}
func mapErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	return err
}
