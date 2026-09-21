package repository

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

func TestDocumentVersionMapping(t *testing.T) {
	documentID, projectID, actorID := uuid.New(), uuid.New(), uuid.New()
	revision := toRevision(revisionModel{
		ID: uuid.NewString(), DocumentID: documentID.String(), ProjectID: projectID.String(),
		CreatedBy: actorID.String(), DocumentVersion: "2.1-beta",
	})
	require.Equal(t, "2.1-beta", revision.DocumentVersion)

	upload := toUpload(uploadModel{
		ID: uuid.NewString(), DocumentID: documentID.String(), ProjectID: projectID.String(),
		RevisionID: uuid.NewString(), CreatedBy: actorID.String(), DocumentVersion: "2.1-beta",
		ProjectVersionID: stringPtr(uuid.NewString()),
	})
	require.Equal(t, "2.1-beta", upload.DocumentVersion)

	model := fromUpload(&domain.Upload{
		ID: uuid.New(), DocumentID: documentID, ProjectID: projectID, RevisionID: uuid.New(),
		CreatedBy: actorID, Scope: domain.Scope{VersionID: uuidPtr(uuid.New())},
		DocumentVersion: "2.1-beta",
	})
	require.Equal(t, "2.1-beta", model.DocumentVersion)
}

func stringPtr(value string) *string     { return &value }
func uuidPtr(value uuid.UUID) *uuid.UUID { return &value }
