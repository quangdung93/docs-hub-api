// Package domain chứa entity và hợp đồng repository của module project.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
)

const (
	StatusActive = "active"
	StatusDraft  = "draft"
	RoleOwner    = "owner"
	RoleEditor   = "editor"
	RoleViewer   = "viewer"
)

// Project là aggregate gốc của kho tri thức. RAGFlowDatasetID là reference nội
// bộ, không được trả cho frontend; frontend chỉ cần trạng thái đồng bộ.
type Project struct {
	ID                uuid.UUID `json:"id"`
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Status            string    `json:"status"`
	OwnerID           uuid.UUID `json:"owner_id"`
	Version           int       `json:"version"`
	RAGFlowDatasetID  string    `json:"-"`
	RAGFlowSyncStatus string    `json:"ragflow_sync_status"`
	RAGFlowLastError  string    `json:"ragflow_last_error,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ProjectVersion là một mốc tài liệu trong timeline của project. Version mới
// luôn bắt đầu ở draft và chưa có ReleasedAt.
type ProjectVersion struct {
	ID         uuid.UUID  `json:"id"`
	ProjectID  uuid.UUID  `json:"project_id"`
	Label      string     `json:"label"`
	SequenceNo int64      `json:"sequence_no"`
	Status     string     `json:"status"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	CreatedBy  uuid.UUID  `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateParams gom dữ liệu được ghi atomically khi tạo project.
type CreateParams struct {
	Project   *Project
	RequestID string
}

// CreateVersionParams chứa version và metadata audit được ghi cùng transaction.
type CreateVersionParams struct {
	Version   *ProjectVersion
	RequestID string
}

// Repository là persistence port của project usecase.
type Repository interface {
	CodeExists(ctx context.Context, code string) (bool, error)
	Create(ctx context.Context, params CreateParams) error
	MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error)
	CreateVersion(ctx context.Context, params CreateVersionParams) error
	ListVersions(ctx context.Context, projectID uuid.UUID, page pagination.Query) ([]ProjectVersion, int64, error)
}

var (
	ErrDuplicateCode         = errors.New("project repository: mã project đã tồn tại")
	ErrDuplicateVersionLabel = errors.New("project repository: label version đã tồn tại")
	ErrProjectNotFound       = errors.New("project repository: project không tồn tại")
)
