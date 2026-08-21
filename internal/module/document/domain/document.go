// Package domain chứa entity và hợp đồng nghiệp vụ quản lý tài liệu.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
)

var (
	ErrNotFound = errors.New("không tìm thấy tài liệu")
	ErrConflict = errors.New("xung đột phiên bản tài liệu")
)

type Scope struct {
	VersionID       *uuid.UUID `json:"project_version_id,omitempty"`
	ChangeRequestID *uuid.UUID `json:"change_request_id,omitempty"`
}

func (s Scope) Valid() bool { return (s.VersionID == nil) != (s.ChangeRequestID == nil) }

type Document struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	CreatedBy   uuid.UUID `json:"created_by"`
	Title       string    `json:"title"`
	Key         string    `json:"document_key"`
	Description string    `json:"description"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Revision struct {
	ID                uuid.UUID  `json:"id"`
	DocumentID        uuid.UUID  `json:"document_id"`
	ProjectID         uuid.UUID  `json:"project_id"`
	CreatedBy         uuid.UUID  `json:"created_by"`
	Scope             Scope      `json:"scope"`
	RevisionNo        int        `json:"revision_no"`
	FileName          string     `json:"file_name"`
	MediaType         string     `json:"media_type"`
	SHA256            string     `json:"sha256"`
	ObjectKey         string     `json:"-"`
	Status            string     `json:"status"`
	SizeBytes         int64      `json:"size_bytes"`
	ErrorCode         string     `json:"error_code,omitempty"`
	ErrorDetail       string     `json:"error_detail,omitempty"`
	RAGFlowDocumentID string     `json:"-"`
	RAGFlowSyncStatus string     `json:"ragflow_sync_status,omitempty"`
	RAGFlowLastError  string     `json:"ragflow_last_error,omitempty"`
	RAGFlowSyncedAt   *time.Time `json:"ragflow_synced_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Upload struct {
	ID, ProjectID, DocumentID, RevisionID, CreatedBy                   uuid.UUID
	Scope                                                              Scope
	Title, Description, FileName, MediaType, SHA256, ObjectKey, Status string
	SizeBytes                                                          int64
	ExpiresAt                                                          time.Time
}

type Filter struct {
	Query, Status, MediaType   string
	VersionID, ChangeRequestID *uuid.UUID
}

type CreateRevisionParams struct {
	DocumentID, RevisionID, ProjectID, ActorID                 uuid.UUID
	Scope                                                      Scope
	Title, Description, FileName, MediaType, SHA256, ObjectKey string
	SizeBytes                                                  int64
}

type Repository interface {
	MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error)
	ScopeExists(ctx context.Context, projectID uuid.UUID, scope Scope) (bool, error)
	CreateRevision(ctx context.Context, in CreateRevisionParams) (*Document, *Revision, error)
	CreateUpload(ctx context.Context, upload *Upload) error
	FindUpload(ctx context.Context, projectID, uploadID uuid.UUID) (*Upload, error)
	CompleteUpload(ctx context.Context, upload *Upload) (*Document, *Revision, error)
	List(ctx context.Context, projectID uuid.UUID, filter Filter, page pagination.Query) ([]Document, int64, error)
	FindDocument(ctx context.Context, projectID, documentID uuid.UUID) (*Document, []Revision, error)
	FindRevision(ctx context.Context, projectID, documentID, revisionID uuid.UUID) (*Revision, error)
	Update(ctx context.Context, projectID, documentID uuid.UUID, title, description string, version int) (*Document, error)
	Retry(ctx context.Context, projectID, documentID, revisionID, actorID uuid.UUID) error
	SoftDelete(ctx context.Context, projectID, documentID, actorID uuid.UUID) error
}
