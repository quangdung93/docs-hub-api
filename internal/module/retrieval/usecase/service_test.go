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
	resolved  []domain.ResolvedScope
}

func (f fakeRepo) MemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return f.role, nil
}
func (f fakeRepo) ResolveScope(context.Context, uuid.UUID, domain.Scope) ([]domain.ResolvedScope, error) {
	if !f.scope {
		return nil, nil
	}
	return f.resolved, nil
}
func (f fakeRepo) DatasetID(context.Context, uuid.UUID) (string, error) { return f.datasetID, nil }
func (f fakeRepo) RevisionRefs(context.Context, uuid.UUID, domain.Scope) ([]domain.RevisionRef, error) {
	return f.refs, nil
}

type fakeRetriever struct {
	input port.RAGRetrievalRequest
	calls []port.RAGRetrievalRequest
	data  port.RAGRetrievalResult
}

func (f *fakeRetriever) Retrieve(_ context.Context, input port.RAGRetrievalRequest) (port.RAGRetrievalResult, error) {
	f.input = input
	f.calls = append(f.calls, input)
	return f.data, nil
}

func TestRetrieve_TruyHoiRiengTungScopeDeBaoDamCoverage(t *testing.T) {
	t.Parallel()
	projectID, actorID := uuid.New(), uuid.New()
	version1, version2 := uuid.New(), uuid.New()
	document1, document2 := uuid.New(), uuid.New()
	repo := fakeRepo{
		role: "viewer", scope: true, datasetID: "ds-1",
		resolved: []domain.ResolvedScope{
			{ID: version1, Type: "version", Label: "v1"},
			{ID: version2, Type: "version", Label: "v2"},
		},
		refs: []domain.RevisionRef{
			{DocumentID: document1, RevisionID: uuid.New(), FileName: "v1.md",
				Scope: domain.ResolvedScope{ID: version1, Type: "version", Label: "v1"}, RAGFlowDocumentID: "remote-v1"},
			{DocumentID: document2, RevisionID: uuid.New(), FileName: "v2.md",
				Scope: domain.ResolvedScope{ID: version2, Type: "version", Label: "v2"}, RAGFlowDocumentID: "remote-v2"},
		},
	}
	remote := &fakeRetriever{data: port.RAGRetrievalResult{Chunks: []port.RAGChunk{
		{ID: "chunk-v1", DatasetID: "ds-1", DocumentID: "remote-v1", Content: "version one"},
		{ID: "chunk-v2", DatasetID: "ds-1", DocumentID: "remote-v2", Content: "version two"},
	}}}
	service := New(repo, remote, false)
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})

	result, err := service.Retrieve(ctx, Input{
		ProjectID: projectID, Query: "Thay đổi thế nào?",
		Scope: domain.Scope{Mode: domain.ScopeVersions, VersionIDs: []uuid.UUID{version1, version2}},
	})

	require.NoError(t, err)
	require.Len(t, remote.calls, 2)
	require.Equal(t, []string{"remote-v1"}, remote.calls[0].DocumentIDs)
	require.Equal(t, []string{"remote-v2"}, remote.calls[1].DocumentIDs)
	require.Len(t, result.Citations, 2)
	require.Equal(t, "S1", result.Citations[0].Key)
	require.Equal(t, "S2", result.Citations[1].Key)
}

func TestRetrieve_UsesOnlyAuthorizedScopeAndMapsLocalCitation(t *testing.T) {
	t.Parallel()
	projectID, actorID, versionID := uuid.New(), uuid.New(), uuid.New()
	documentID, revisionID := uuid.New(), uuid.New()
	repo := fakeRepo{role: "viewer", scope: true, datasetID: "ds-1",
		resolved: []domain.ResolvedScope{{ID: versionID, Type: "version", Label: "v1"}}, refs: []domain.RevisionRef{{
			DocumentID: documentID, RevisionID: revisionID, Title: "Policy", FileName: "policy.md",
			Scope: domain.ResolvedScope{ID: versionID, Type: "version", Label: "v1"}, RAGFlowDocumentID: "remote-1",
		}}}
	remote := &fakeRetriever{data: port.RAGRetrievalResult{Chunks: []port.RAGChunk{
		{ID: "chunk-1", DatasetID: "ds-1", DocumentID: "remote-1", Content: "allowed", Similarity: 0.9},
		{ID: "chunk-2", DatasetID: "ds-1", DocumentID: "remote-other", Content: "must be filtered"},
		{ID: "chunk-3", DatasetID: "ds-other", DocumentID: "remote-1", Content: "must be filtered"},
	}}}
	service := New(repo, remote, false)
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})
	result, err := service.Retrieve(ctx, Input{
		ProjectID: projectID, Scope: domain.Scope{Mode: domain.ScopeVersions, VersionIDs: []uuid.UUID{versionID}},
		Query: "What is policy?",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"ds-1"}, remote.input.DatasetIDs)
	require.Equal(t, []string{"remote-1"}, remote.input.DocumentIDs)
	require.Len(t, result.Citations, 1)
	require.Equal(t, documentID, result.Citations[0].DocumentID)
	require.Equal(t, revisionID, result.Citations[0].DocumentRevisionID)
	require.Equal(t, "S1", result.Citations[0].Key)
	require.Equal(t, "v1", result.Citations[0].ScopeLabel)
}

func TestRetrieve_RejectsUserOutsideProjectBeforeRemoteCall(t *testing.T) {
	t.Parallel()
	actorID, versionID := uuid.New(), uuid.New()
	remote := &fakeRetriever{}
	service := New(fakeRepo{role: "", scope: true}, remote, false)
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})
	_, err := service.Retrieve(ctx, Input{
		ProjectID: uuid.New(), Scope: domain.Scope{Mode: domain.ScopeVersions, VersionIDs: []uuid.UUID{versionID}},
		Query: "question",
	})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 403, technical.HTTPStatus)
	require.Empty(t, remote.input.Question)
}
