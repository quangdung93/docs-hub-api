// Package domain định nghĩa dữ liệu và repository cho retrieval có ACL/scope.
package domain

import (
	"context"

	"github.com/google/uuid"
)

type Scope struct {
	VersionID       *uuid.UUID
	ChangeRequestID *uuid.UUID
}

func (s Scope) Valid() bool { return (s.VersionID == nil) != (s.ChangeRequestID == nil) }

type RevisionRef struct {
	DocumentID        uuid.UUID
	RevisionID        uuid.UUID
	Title             string
	RAGFlowDocumentID string
}

type Repository interface {
	MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error)
	ScopeExists(ctx context.Context, projectID uuid.UUID, scope Scope) (bool, error)
	DatasetID(ctx context.Context, projectID uuid.UUID) (string, error)
	RevisionRefs(ctx context.Context, projectID uuid.UUID, scope Scope) ([]RevisionRef, error)
}
