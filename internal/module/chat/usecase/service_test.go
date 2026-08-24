package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/chat/domain"
	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

type fakeRepo struct {
	role         string
	conversation *domain.Conversation
	saved        *domain.Exchange
	chatID       string
}

func (f *fakeRepo) MemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return f.role, nil
}
func (f *fakeRepo) Create(_ context.Context, conversation *domain.Conversation) error {
	f.conversation = conversation
	return nil
}
func (*fakeRepo) List(context.Context, uuid.UUID, uuid.UUID, pagination.Query) ([]domain.Conversation, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) Get(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Conversation, error) {
	return f.conversation, nil
}
func (f *fakeRepo) SaveExchange(_ context.Context, _ uuid.UUID, exchange domain.Exchange) (*domain.Message, error) {
	f.saved = &exchange
	return &domain.Message{}, nil
}
func (f *fakeRepo) RAGFlowChatID(context.Context, uuid.UUID) (string, error) { return f.chatID, nil }
func (f *fakeRepo) SaveRAGFlowChatID(_ context.Context, _ uuid.UUID, chatID string) (string, error) {
	f.chatID = chatID
	return chatID, nil
}

type fakeScopeRepo struct {
	resolved []retrievaldomain.ResolvedScope
	refs     []retrievaldomain.RevisionRef
	dataset  string
	scope    retrievaldomain.Scope
}

func (f *fakeScopeRepo) ResolveScope(_ context.Context, _ uuid.UUID, scope retrievaldomain.Scope) ([]retrievaldomain.ResolvedScope, error) {
	f.scope = scope
	return f.resolved, nil
}
func (f *fakeScopeRepo) DatasetID(context.Context, uuid.UUID) (string, error) { return f.dataset, nil }
func (f *fakeScopeRepo) RevisionRefs(_ context.Context, _ uuid.UUID, scope retrievaldomain.Scope) ([]retrievaldomain.RevisionRef, error) {
	f.scope = scope
	return f.refs, nil
}

type fakeRAG struct {
	completionInput port.RAGChatCompletionRequest
	completion      port.RAGChatCompletionResult
	metadata        map[string]string
	metadataIDs     []string
	createChatCalls int
}

func (*fakeRAG) Health(context.Context) error { return nil }
func (*fakeRAG) CreateDataset(context.Context, string, string) (port.RAGDataset, error) {
	return port.RAGDataset{}, nil
}
func (*fakeRAG) FindDatasetByName(context.Context, string) (*port.RAGDataset, error) { return nil, nil }
func (*fakeRAG) UpdateDataset(context.Context, string, string, string) error         { return nil }
func (*fakeRAG) DeleteDatasets(context.Context, []string) error                      { return nil }
func (*fakeRAG) UploadDocument(context.Context, string, port.RAGDocumentFile) (port.RAGDocument, error) {
	return port.RAGDocument{}, nil
}
func (*fakeRAG) GetDocument(context.Context, string, string) (port.RAGDocument, error) {
	return port.RAGDocument{}, nil
}
func (*fakeRAG) FindDocumentByName(context.Context, string, string) (*port.RAGDocument, error) {
	return nil, nil
}
func (*fakeRAG) StartParsing(context.Context, string, []string) error    { return nil }
func (*fakeRAG) StopParsing(context.Context, string, []string) error     { return nil }
func (*fakeRAG) DeleteDocuments(context.Context, string, []string) error { return nil }
func (f *fakeRAG) UpdateDocumentMetadata(_ context.Context, _ string, ids []string, metadata map[string]string) error {
	f.metadataIDs = append([]string(nil), ids...)
	f.metadata = metadata
	return nil
}
func (*fakeRAG) Retrieve(context.Context, port.RAGRetrievalRequest) (port.RAGRetrievalResult, error) {
	return port.RAGRetrievalResult{}, nil
}
func (f *fakeRAG) CreateChat(_ context.Context, name string, datasetIDs []string) (port.RAGChat, error) {
	f.createChatCalls++
	return port.RAGChat{ID: "chat-1", Name: name, DatasetIDs: datasetIDs}, nil
}
func (*fakeRAG) FindChatByName(context.Context, string) (*port.RAGChat, error) { return nil, nil }
func (*fakeRAG) UpdateChatDatasets(context.Context, string, []string) error    { return nil }
func (f *fakeRAG) CompleteChat(_ context.Context, input port.RAGChatCompletionRequest) (port.RAGChatCompletionResult, error) {
	f.completionInput = input
	return f.completion, nil
}

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

func TestAsk_DungRAGFlowChatVaMapCitationLocal(t *testing.T) {
	t.Parallel()
	projectID, actorID, conversationID := uuid.New(), uuid.New(), uuid.New()
	versionID, documentID, revisionID := uuid.New(), uuid.New(), uuid.New()
	scope := retrievaldomain.Scope{Mode: retrievaldomain.ScopeVersions, VersionIDs: []uuid.UUID{versionID}}
	resolved := retrievaldomain.ResolvedScope{ID: versionID, Type: "version", Label: "v1"}
	ref := retrievaldomain.RevisionRef{
		DocumentID: documentID, RevisionID: revisionID, Title: "Requirements",
		FileName: "requirements.md", Scope: resolved, RAGFlowDocumentID: "remote-doc-1",
	}
	repo := &fakeRepo{role: "viewer", conversation: &domain.Conversation{
		ID: conversationID, ProjectID: projectID, UserID: actorID,
		Messages: []domain.Message{{Role: "user", Content: "Câu trước"}},
	}}
	scopes := &fakeScopeRepo{resolved: []retrievaldomain.ResolvedScope{resolved}, refs: []retrievaldomain.RevisionRef{ref}, dataset: "ds-1"}
	rag := &fakeRAG{completion: port.RAGChatCompletionResult{
		Content: "Quy trình đã được cập nhật.", Model: "qwen@ragflow",
		References: []port.RAGChunk{{ID: "chunk-1", DatasetID: "ds-1", DocumentID: "remote-doc-1", Content: "Quy trình mới."}},
	}}
	service := New(repo, scopes, rag, fixedClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})

	answer, err := service.Ask(ctx, AskInput{
		ProjectID: projectID, ConversationID: conversationID, Question: "Quy trình thế nào?", Scope: &scope,
	})

	require.NoError(t, err)
	require.True(t, answer.Grounded)
	require.Equal(t, "Quy trình đã được cập nhật.", answer.Answer)
	require.Equal(t, "qwen@ragflow", repo.saved.Model)
	require.Equal(t, "ragflow-chat-v1", repo.saved.PromptVersion)
	require.Len(t, repo.saved.Citations, 1)
	require.Equal(t, documentID, repo.saved.Citations[0].DocumentID)
	require.Equal(t, []string{"remote-doc-1"}, rag.metadataIDs)
	require.Equal(t, versionID.String(), rag.metadata[scopeMetadataKey])
	require.Len(t, rag.completionInput.MetadataConditions, 1)
	require.Equal(t, versionID.String(), rag.completionInput.MetadataConditions[0].Value)
	require.Len(t, rag.completionInput.Messages, 2)
	require.Equal(t, 1, rag.createChatCalls)
	require.Equal(t, "chat-1", repo.chatID)
}

func TestAsk_KhongTruyenScopeThiDungToanBoVersionVaChangeRequest(t *testing.T) {
	t.Parallel()
	projectID, actorID := uuid.New(), uuid.New()
	versionID, changeID := uuid.New(), uuid.New()
	version := retrievaldomain.ResolvedScope{ID: versionID, Type: "version", Label: "v1"}
	change := retrievaldomain.ResolvedScope{ID: changeID, Type: "change_request", Label: "CR-1"}
	repo := &fakeRepo{role: "viewer", chatID: "chat-existing", conversation: &domain.Conversation{
		ID: uuid.New(), ProjectID: projectID,
	}}
	scopes := &fakeScopeRepo{dataset: "ds-1", refs: []retrievaldomain.RevisionRef{
		{DocumentID: uuid.New(), RevisionID: uuid.New(), Scope: version, RAGFlowDocumentID: "doc-v1"},
		{DocumentID: uuid.New(), RevisionID: uuid.New(), Scope: change, RAGFlowDocumentID: "doc-cr1"},
	}}
	rag := &fakeRAG{completion: port.RAGChatCompletionResult{Content: "Timeline", Model: "ragflow-model"}}
	service := New(repo, scopes, rag, fixedClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})

	answer, err := service.Ask(ctx, AskInput{
		ProjectID: projectID, ConversationID: repo.conversation.ID, Question: "Có gì thay đổi?",
	})

	require.NoError(t, err)
	require.Equal(t, retrievaldomain.ScopeAll, scopes.scope.Mode)
	require.Equal(t, "evolution", answer.Intent)
	require.Equal(t, []retrievaldomain.ResolvedScope{version, change}, answer.ResolvedScope)
	require.Empty(t, rag.completionInput.MetadataConditions)
	require.Equal(t, retrievaldomain.ScopeAll, repo.saved.Scope.Mode)
}

func TestAsk_KhongCoRevisionThiKhongGoiRAGFlowChat(t *testing.T) {
	t.Parallel()
	projectID, actorID := uuid.New(), uuid.New()
	repo := &fakeRepo{role: "viewer", conversation: &domain.Conversation{ID: uuid.New(), ProjectID: projectID}}
	scopes := &fakeScopeRepo{}
	rag := &fakeRAG{}
	service := New(repo, scopes, rag, fixedClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})

	answer, err := service.Ask(ctx, AskInput{
		ProjectID: projectID, ConversationID: repo.conversation.ID, Question: "Câu hỏi",
	})

	require.NoError(t, err)
	require.False(t, answer.Grounded)
	require.Contains(t, answer.Answer, "Không tìm thấy")
	require.Empty(t, rag.completionInput.ChatID)
}
