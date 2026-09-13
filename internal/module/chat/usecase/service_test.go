package usecase

import (
	"context"
	"sync"
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
	if scope.Mode == retrievaldomain.ScopeAll {
		return nil, nil
	}
	allowed := make(map[uuid.UUID]struct{})
	for _, id := range scope.VersionIDs {
		allowed[id] = struct{}{}
	}
	for _, id := range scope.ChangeRequestIDs {
		allowed[id] = struct{}{}
	}
	out := make([]retrievaldomain.ResolvedScope, 0, len(allowed))
	for _, resolved := range f.resolved {
		if _, ok := allowed[resolved.ID]; ok {
			out = append(out, resolved)
		}
	}
	return out, nil
}
func (f *fakeScopeRepo) DatasetID(context.Context, uuid.UUID) (string, error) { return f.dataset, nil }
func (f *fakeScopeRepo) VersionScopes(context.Context, uuid.UUID) ([]retrievaldomain.ResolvedScope, error) {
	out := make([]retrievaldomain.ResolvedScope, 0, len(f.resolved))
	for _, resolved := range f.resolved {
		if resolved.Type == "version" {
			out = append(out, resolved)
		}
	}
	return out, nil
}
func (f *fakeScopeRepo) RevisionRefs(_ context.Context, _ uuid.UUID, scope retrievaldomain.Scope) ([]retrievaldomain.RevisionRef, error) {
	f.scope = scope
	if scope.Mode == retrievaldomain.ScopeAll {
		return f.refs, nil
	}
	allowed := make(map[uuid.UUID]struct{})
	for _, id := range scope.VersionIDs {
		allowed[id] = struct{}{}
	}
	for _, id := range scope.ChangeRequestIDs {
		allowed[id] = struct{}{}
	}
	out := make([]retrievaldomain.RevisionRef, 0, len(f.refs))
	for _, ref := range f.refs {
		if _, ok := allowed[ref.Scope.ID]; ok {
			out = append(out, ref)
		}
	}
	return out, nil
}

type fakeRAG struct {
	mu               sync.Mutex
	completionInput  port.RAGChatCompletionRequest
	completionInputs []port.RAGChatCompletionRequest
	completion       port.RAGChatCompletionResult
	completions      []port.RAGChatCompletionResult
	retrieval        port.RAGRetrievalResult
	retrievalInputs  []port.RAGRetrievalRequest
	metadata         map[string]string
	metadataIDs      []string
	createChatCalls  int
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
func (f *fakeRAG) Retrieve(_ context.Context, input port.RAGRetrievalRequest) (port.RAGRetrievalResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retrievalInputs = append(f.retrievalInputs, input)
	return f.retrieval, nil
}
func (f *fakeRAG) CreateChat(_ context.Context, name string, datasetIDs []string) (port.RAGChat, error) {
	f.createChatCalls++
	return port.RAGChat{ID: "chat-1", Name: name, DatasetIDs: datasetIDs}, nil
}
func (*fakeRAG) FindChatByName(context.Context, string) (*port.RAGChat, error) { return nil, nil }
func (*fakeRAG) UpdateChatDatasets(context.Context, string, []string) error    { return nil }
func (f *fakeRAG) CompleteChat(_ context.Context, input port.RAGChatCompletionRequest) (port.RAGChatCompletionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completionInput = input
	f.completionInputs = append(f.completionInputs, input)
	if len(f.completions) > 0 {
		result := f.completions[0]
		f.completions = f.completions[1:]
		return result, nil
	}
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
	require.Equal(t, "ragflow-chat-v2-planner", repo.saved.PromptVersion)
	require.Len(t, repo.saved.Citations, 1)
	require.Equal(t, documentID, repo.saved.Citations[0].DocumentID)
	require.Equal(t, []string{"remote-doc-1"}, rag.metadataIDs)
	require.Equal(t, versionID.String(), rag.metadata[scopeMetadataKey])
	require.Len(t, rag.completionInput.MetadataConditions, 1)
	require.Equal(t, versionID.String(), rag.completionInput.MetadataConditions[0].Value)
	require.Len(t, rag.completionInput.Messages, 3)
	require.Equal(t, "system", rag.completionInput.Messages[0].Role)
	// Hỏi-đáp CẦN trích dẫn: mapRAGCitations dựng danh sách nguồn từ References
	// để trả cho người đọc. Khác module report vốn phải tắt vì "[ID:n]" phá parse.
	require.True(t, rag.completionInput.WantReference)
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
	scopes := &fakeScopeRepo{dataset: "ds-1", resolved: []retrievaldomain.ResolvedScope{version, change}, refs: []retrievaldomain.RevisionRef{
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
	require.Equal(t, retrievaldomain.ScopeVersions, scopes.scope.Mode)
	require.Equal(t, "evolution", answer.Intent)
	require.Equal(t, []retrievaldomain.ResolvedScope{version}, answer.ResolvedScope)
	require.Len(t, rag.completionInput.MetadataConditions, 1)
	require.Equal(t, retrievaldomain.ScopeVersions, repo.saved.Scope.Mode)
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

func TestAsk_AIPlannerChonVersionMoiNhatVaTachNhieuTruyVan(t *testing.T) {
	t.Parallel()
	projectID, actorID := uuid.New(), uuid.New()
	v1ID, v2ID := uuid.New(), uuid.New()
	v1 := retrievaldomain.ResolvedScope{ID: v1ID, Type: "version", Label: "v1"}
	v2 := retrievaldomain.ResolvedScope{ID: v2ID, Type: "version", Label: "v2"}
	oldRef := retrievaldomain.RevisionRef{
		DocumentID: uuid.New(), RevisionID: uuid.New(), FileName: "feature-a-v1.md",
		Scope: v1, RAGFlowDocumentID: "doc-v1",
	}
	latestRef := retrievaldomain.RevisionRef{
		DocumentID: uuid.New(), RevisionID: uuid.New(), FileName: "feature-a-v2.md",
		Scope: v2, RAGFlowDocumentID: "doc-v2",
	}
	repo := &fakeRepo{role: "viewer", chatID: "chat-1", conversation: &domain.Conversation{
		ID: uuid.New(), ProjectID: projectID, UserID: actorID,
	}}
	scopes := &fakeScopeRepo{resolved: []retrievaldomain.ResolvedScope{v1, v2},
		refs: []retrievaldomain.RevisionRef{oldRef, latestRef}, dataset: "ds-1"}
	rag := &fakeRAG{
		completions: []port.RAGChatCompletionResult{
			{Content: `{"intent":"current_state","scope":"latest_version","version_label":"","queries":["Chức năng A xử lý gì?","Mục đích của chức năng A là gì?"]}`},
			{Content: "Ở v2, chức năng A xử lý dữ liệu để kiểm tra đầu vào.", Model: "qwen@ragflow",
				References: []port.RAGChunk{{ID: "final-chunk", DatasetID: "ds-1", DocumentID: "doc-v2"}}},
		},
		retrieval: port.RAGRetrievalResult{Chunks: []port.RAGChunk{
			{ID: "evidence-chunk", DatasetID: "ds-1", DocumentID: "doc-v2", Content: "A kiểm tra dữ liệu đầu vào."},
		}},
	}
	service := New(repo, scopes, rag, fixedClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})

	answer, err := service.Ask(ctx, AskInput{
		ProjectID: projectID, ConversationID: repo.conversation.ID,
		Question: "Chức năng A đang xử lý gì và mục đích là gì?",
	})

	require.NoError(t, err)
	require.Equal(t, "current_state", answer.Intent)
	require.Equal(t, []retrievaldomain.ResolvedScope{v2}, answer.ResolvedScope)
	require.Equal(t, retrievaldomain.ScopeVersions, repo.saved.Scope.Mode)
	require.Equal(t, []uuid.UUID{v2ID}, repo.saved.Scope.VersionIDs)
	require.Len(t, rag.retrievalInputs, 2)
	for _, input := range rag.retrievalInputs {
		require.Equal(t, []string{"doc-v2"}, input.DocumentIDs)
	}
	require.Len(t, rag.completionInputs, 2, "một lượt planner và một lượt trả lời")
	require.Equal(t, "system", rag.completionInputs[0].Messages[0].Role)
	require.Equal(t, "system", rag.completionInputs[1].Messages[0].Role)
	require.Contains(t, rag.completionInputs[1].Messages[len(rag.completionInputs[1].Messages)-1].Content,
		"Các truy vấn phụ cần tổng hợp")
	require.Len(t, answer.Citations, 2)
}

func TestAsk_AIPlannerTongHopThayDoiQuaTatCaVersion(t *testing.T) {
	t.Parallel()
	projectID, actorID := uuid.New(), uuid.New()
	v1ID, v2ID := uuid.New(), uuid.New()
	v1 := retrievaldomain.ResolvedScope{ID: v1ID, Type: "version", Label: "v1"}
	v2 := retrievaldomain.ResolvedScope{ID: v2ID, Type: "version", Label: "v2"}
	repo := &fakeRepo{role: "viewer", chatID: "chat-1", conversation: &domain.Conversation{
		ID: uuid.New(), ProjectID: projectID, UserID: actorID,
	}}
	scopes := &fakeScopeRepo{resolved: []retrievaldomain.ResolvedScope{v1, v2}, dataset: "ds-1",
		refs: []retrievaldomain.RevisionRef{
			{DocumentID: uuid.New(), RevisionID: uuid.New(), FileName: "v1.md", Scope: v1, RAGFlowDocumentID: "doc-v1"},
			{DocumentID: uuid.New(), RevisionID: uuid.New(), FileName: "v2.md", Scope: v2, RAGFlowDocumentID: "doc-v2"},
		}}
	rag := &fakeRAG{completions: []port.RAGChatCompletionResult{
		{Content: `{"intent":"evolution","scope":"all_versions","version_label":"","queries":["Chức năng A thay đổi thế nào qua các version?"]}`},
		{Content: "v1 tiếp nhận thủ công; v2 tự động kiểm tra.", Model: "qwen@ragflow"},
	}}
	service := New(repo, scopes, rag, fixedClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})

	answer, err := service.Ask(ctx, AskInput{
		ProjectID: projectID, ConversationID: repo.conversation.ID,
		Question: "Chức năng A đã được thay đổi như thế nào?",
	})

	require.NoError(t, err)
	require.Equal(t, "evolution", answer.Intent)
	require.Equal(t, []retrievaldomain.ResolvedScope{v1, v2}, answer.ResolvedScope)
	require.Equal(t, []uuid.UUID{v1ID, v2ID}, repo.saved.Scope.VersionIDs)
	require.Len(t, rag.completionInput.MetadataConditions, 2)
	require.Contains(t, rag.completionInput.Messages[0].Content, "trình tự version cũ đến mới")
}

func TestAsk_VersionMoiNhatChuaIndexKhongDuocLuiVeVersionCu(t *testing.T) {
	t.Parallel()
	projectID, actorID := uuid.New(), uuid.New()
	v1 := retrievaldomain.ResolvedScope{ID: uuid.New(), Type: "version", Label: "v1"}
	v2 := retrievaldomain.ResolvedScope{ID: uuid.New(), Type: "version", Label: "v2"}
	repo := &fakeRepo{role: "viewer", chatID: "chat-1", conversation: &domain.Conversation{
		ID: uuid.New(), ProjectID: projectID, UserID: actorID,
	}}
	scopes := &fakeScopeRepo{resolved: []retrievaldomain.ResolvedScope{v1, v2}, dataset: "ds-1",
		refs: []retrievaldomain.RevisionRef{{
			DocumentID: uuid.New(), RevisionID: uuid.New(), FileName: "v1.md",
			Scope: v1, RAGFlowDocumentID: "doc-v1",
		}}}
	rag := &fakeRAG{completion: port.RAGChatCompletionResult{Content: `{
		"intent":"current_state","scope":"latest_version","version_label":"","queries":["Chức năng A hiện tại làm gì?"]}`}}
	service := New(repo, scopes, rag, fixedClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})

	answer, err := service.Ask(ctx, AskInput{
		ProjectID: projectID, ConversationID: repo.conversation.ID,
		Question: "Chức năng A hiện tại làm gì?",
	})

	require.NoError(t, err)
	require.False(t, answer.Grounded)
	require.Contains(t, answer.Answer, "Không tìm thấy đủ thông tin")
	require.Equal(t, []retrievaldomain.ResolvedScope{v2}, answer.ResolvedScope)
	require.Equal(t, []uuid.UUID{v2.ID}, repo.saved.Scope.VersionIDs)
	require.Len(t, rag.completionInputs, 1, "không hỏi version cũ để lấp dữ liệu thiếu của version mới")
}
