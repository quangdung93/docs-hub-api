// Package domain định nghĩa dữ liệu và repository cho retrieval có ACL/scope.
package domain

import (
	"context"

	"github.com/google/uuid"
)

type Scope struct {
	Mode             string      `json:"mode"`
	VersionIDs       []uuid.UUID `json:"version_ids,omitempty"`
	ChangeRequestIDs []uuid.UUID `json:"change_request_ids,omitempty"`
}

const (
	ScopeVersions       = "versions"
	ScopeChangeRequests = "change_requests"
	ScopeAll            = "all"
)

func (s Scope) Valid() bool {
	switch s.Mode {
	case ScopeVersions:
		return len(s.VersionIDs) > 0 && len(s.ChangeRequestIDs) == 0
	case ScopeChangeRequests:
		return len(s.ChangeRequestIDs) > 0 && len(s.VersionIDs) == 0
	case ScopeAll:
		return len(s.VersionIDs) == 0 && len(s.ChangeRequestIDs) == 0
	default:
		return false
	}
}

type ResolvedScope struct {
	ID    uuid.UUID `json:"id"`
	Type  string    `json:"type"`
	Label string    `json:"label"`
}

type RevisionRef struct {
	DocumentID        uuid.UUID
	RevisionID        uuid.UUID
	Title             string
	FileName          string
	Scope             ResolvedScope
	RAGFlowDocumentID string
}

type Repository interface {
	MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error)
	ResolveScope(ctx context.Context, projectID uuid.UUID, scope Scope) ([]ResolvedScope, error)
	DatasetID(ctx context.Context, projectID uuid.UUID) (string, error)
	RevisionRefs(ctx context.Context, projectID uuid.UUID, scope Scope) ([]RevisionRef, error)
}
