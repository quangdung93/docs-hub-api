package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

type fakeRepo struct {
	role      string
	scope     bool
	datasetID string
	refs      []domain.RevisionRef
}

func (f fakeRepo) MemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return f.role, nil
}
func (f fakeRepo) ScopeExists(context.Context, uuid.UUID, domain.Scope) (bool, error) {
	return f.scope, nil
}
func (f fakeRepo) DatasetID(context.Context, uuid.UUID) (string, error) { return f.datasetID, nil }
func (f fakeRepo) RevisionRefs(context.Context, uuid.UUID, domain.Scope) ([]domain.RevisionRef, error) {
	return f.refs, nil
}

type fakeRetriever struct {
	input port.RAGRetrievalRequest
	data  port.RAGRetrievalResult
}

func (f *fakeRetriever) Retrieve(_ context.Context, input port.RAGRetrievalRequest) (port.RAGRetrievalResult, error) {
	f.input = input
	return f.data, nil
}

func TestRetrieve_UsesOnlyAuthorizedScopeAndMapsLocalCitation(t *testing.T) {
	t.Parallel()
	projectID, actorID, versionID := uuid.New(), uuid.New(), uuid.New()
	documentID, revisionID := uuid.New(), uuid.New()
	repo := fakeRepo{role: "viewer", scope: true, datasetID: "ds-1", refs: []domain.RevisionRef{{
		DocumentID: documentID, RevisionID: revisionID, Title: "Policy", RAGFlowDocumentID: "remote-1",
	}}}
	remote := &fakeRetriever{data: port.RAGRetrievalResult{Chunks: []port.RAGChunk{
		{ID: "chunk-1", DatasetID: "ds-1", DocumentID: "remote-1", Content: "allowed", Similarity: 0.9},
		{ID: "chunk-2", DatasetID: "ds-1", DocumentID: "remote-other", Content: "must be filtered"},
		{ID: "chunk-3", DatasetID: "ds-other", DocumentID: "remote-1", Content: "must be filtered"},
	}}}
	service := New(repo, remote, false)
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})
	result, err := service.Retrieve(ctx, Input{
		ProjectID: projectID, Scope: domain.Scope{VersionID: &versionID}, Question: "What is policy?",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"ds-1"}, remote.input.DatasetIDs)
	require.Equal(t, []string{"remote-1"}, remote.input.DocumentIDs)
	require.Len(t, result.Citations, 1)
	require.Equal(t, documentID, result.Citations[0].DocumentID)
	require.Equal(t, revisionID, result.Citations[0].RevisionID)
}

func TestRetrieve_RejectsUserOutsideProjectBeforeRemoteCall(t *testing.T) {
	t.Parallel()
	actorID, versionID := uuid.New(), uuid.New()
	remote := &fakeRetriever{}
	service := New(fakeRepo{role: "", scope: true}, remote, false)
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})
	_, err := service.Retrieve(ctx, Input{
		ProjectID: uuid.New(), Scope: domain.Scope{VersionID: &versionID}, Question: "question",
	})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 403, technical.HTTPStatus)
	require.Empty(t, remote.input.Question)
}
