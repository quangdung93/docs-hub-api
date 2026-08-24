// Package usecase điều phối hội thoại, retrieval và grounding citation.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/chat/domain"
	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

const (
	promptVersion     = "ragflow-chat-v1"
	maxTitleLength    = 255
	maxQuestionLength = 8000
	scopeMetadataKey  = "docs_hub_scope_id"
)

type ScopeRepository interface {
	ResolveScope(ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope) ([]retrievaldomain.ResolvedScope, error)
	DatasetID(ctx context.Context, projectID uuid.UUID) (string, error)
	RevisionRefs(ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope) ([]retrievaldomain.RevisionRef, error)
}

type Service struct {
	repo      domain.Repository
	scopeRepo ScopeRepository
	rag       port.RAGClient
	clock     port.Clock
}

func New(repo domain.Repository, scopeRepo ScopeRepository, rag port.RAGClient, clock port.Clock) *Service {
	return &Service{repo: repo, scopeRepo: scopeRepo, rag: rag, clock: clock}
}

type CreateInput struct {
	ProjectID uuid.UUID
	Title     string
	Scope     *retrievaldomain.Scope
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*domain.Conversation, error) {
	actorID, err := s.authorize(ctx, input.ProjectID)
	if err != nil {
		return nil, err
	}
	input.Title = strings.TrimSpace(input.Title)
	if len([]rune(input.Title)) > maxTitleLength || input.Scope != nil && !input.Scope.Valid() {
		return nil, apperr.BadRequest("Tiêu đề hoặc scope conversation không hợp lệ")
	}
	now := s.clock.Now().UTC()
	conversation := &domain.Conversation{
		ID: uuid.New(), ProjectID: input.ProjectID, UserID: actorID,
		Title: input.Title, ActiveScope: input.Scope, CreatedAt: now, UpdatedAt: now,
	}
	if err = s.repo.Create(ctx, conversation); err != nil {
		return nil, apperr.Database("Không thể tạo conversation").WithCause(err)
	}
	return conversation, nil
}

func (s *Service) List(
	ctx context.Context, projectID uuid.UUID, page pagination.Query,
) ([]domain.Conversation, pagination.Meta, error) {
	actorID, err := s.authorize(ctx, projectID)
	if err != nil {
		return nil, pagination.Meta{}, err
	}
	page = page.Normalize()
	items, total, err := s.repo.List(ctx, projectID, actorID, page)
	if err != nil {
		return nil, pagination.Meta{}, apperr.Database("Không thể đọc conversation").WithCause(err)
	}
	return items, pagination.NewMeta(page.Page, page.Limit, total), nil
}

func (s *Service) Get(ctx context.Context, projectID, conversationID uuid.UUID) (*domain.Conversation, error) {
	actorID, err := s.authorize(ctx, projectID)
	if err != nil {
		return nil, err
	}
	conversation, err := s.repo.Get(ctx, projectID, actorID, conversationID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, apperr.NotFound("NOT_FOUND", "Không tìm thấy conversation").WithCause(err)
	}
	if err != nil {
		return nil, apperr.Database("Không thể đọc conversation").WithCause(err)
	}
	return conversation, nil
}

type AskInput struct {
	ProjectID, ConversationID uuid.UUID
	Question                  string
	Scope                     *retrievaldomain.Scope
}

type Answer struct {
	Answer        string                          `json:"answer"`
	Intent        string                          `json:"intent"`
	ResolvedScope []retrievaldomain.ResolvedScope `json:"resolved_scope"`
	Citations     []domain.Citation               `json:"citations"`
	Grounded      bool                            `json:"grounded"`
}

func (s *Service) Ask(ctx context.Context, input AskInput) (*Answer, error) {
	started := time.Now()
	conversation, err := s.Get(ctx, input.ProjectID, input.ConversationID)
	if err != nil {
		return nil, err
	}
	input.Question = strings.TrimSpace(input.Question)
	if input.Question == "" || len([]rune(input.Question)) > maxQuestionLength {
		return nil, apperr.BadRequest("Câu hỏi rỗng hoặc vượt quá 8000 ký tự")
	}
	scope := input.Scope
	if scope == nil {
		scope = &retrievaldomain.Scope{Mode: retrievaldomain.ScopeAll}
	}
	if !scope.Valid() {
		return nil, apperr.BadRequest("Scope không hợp lệ")
	}
	resolved, refs, err := s.resolveScope(ctx, input.ProjectID, *scope)
	if err != nil {
		return nil, err
	}
	intent := intentFor(*scope)
	if len(refs) == 0 {
		return s.save(ctx, input, *scope, resolved, intent,
			"Không tìm thấy đủ thông tin trong phạm vi tài liệu đã chọn.", "", nil, false, started)
	}
	datasetID, err := s.scopeRepo.DatasetID(ctx, input.ProjectID)
	if err != nil {
		return nil, apperr.Database("Không thể đọc RAGFlow dataset mapping").WithCause(err)
	}
	if datasetID == "" {
		return nil, apperr.External("Project chưa được đồng bộ sang RAGFlow")
	}
	if scope.Mode != retrievaldomain.ScopeAll {
		if err = s.syncScopeMetadata(ctx, datasetID, refs); err != nil {
			return nil, apperr.External("Không thể đồng bộ scope metadata sang RAGFlow").WithCause(err)
		}
	}
	chatID, err := s.ensureChat(ctx, input.ProjectID, datasetID)
	if err != nil {
		return nil, apperr.External("Không thể chuẩn bị RAGFlow chat assistant").WithCause(err)
	}
	result, err := s.rag.CompleteChat(ctx, port.RAGChatCompletionRequest{
		ChatID: chatID, Messages: ragMessages(conversation.Messages, input.Question),
		MetadataLogic: "or", MetadataConditions: scopeConditions(*scope),
	})
	if err != nil {
		return nil, apperr.External("RAGFlow chat không khả dụng").WithCause(err)
	}
	citations := mapRAGCitations(input.ProjectID, datasetID, refs, result.References)
	return s.save(ctx, input, *scope, resolved, intent, result.Content, result.Model,
		citations, len(citations) > 0, started)
}

func (s *Service) save(
	ctx context.Context, input AskInput, scope retrievaldomain.Scope,
	resolved []retrievaldomain.ResolvedScope, intent, answer, model string,
	citations []domain.Citation, grounded bool, started time.Time,
) (*Answer, error) {
	exchange := domain.Exchange{
		Question: input.Question, Answer: answer, Intent: intent, Scope: scope,
		ResolvedScope: resolved, Model: model, PromptVersion: promptVersion,
		LatencyMS: time.Since(started).Milliseconds(), Citations: citations,
	}
	if _, err := s.repo.SaveExchange(ctx, input.ConversationID, exchange); err != nil {
		return nil, apperr.Database("Không thể lưu message").WithCause(err)
	}
	return &Answer{
		Answer: answer, Intent: intent, ResolvedScope: resolved,
		Citations: citations, Grounded: grounded,
	}, nil
}

func (s *Service) resolveScope(
	ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope,
) ([]retrievaldomain.ResolvedScope, []retrievaldomain.RevisionRef, error) {
	resolved, err := s.scopeRepo.ResolveScope(ctx, projectID, scope)
	if err != nil {
		return nil, nil, apperr.Database("Không thể kiểm tra scope").WithCause(err)
	}
	expected := len(scope.VersionIDs) + len(scope.ChangeRequestIDs)
	if scope.Mode != retrievaldomain.ScopeAll && len(resolved) != expected {
		return nil, nil, apperr.NotFound("NOT_FOUND", "Không tìm thấy version hoặc change request")
	}
	refs, err := s.scopeRepo.RevisionRefs(ctx, projectID, scope)
	if err != nil {
		return nil, nil, apperr.Database("Không thể đọc revision mapping").WithCause(err)
	}
	if scope.Mode == retrievaldomain.ScopeAll {
		resolved = scopesFrom(refs)
	}
	return resolved, refs, nil
}

func (s *Service) syncScopeMetadata(
	ctx context.Context, datasetID string, refs []retrievaldomain.RevisionRef,
) error {
	type group struct {
		scope retrievaldomain.ResolvedScope
		ids   []string
	}
	groups := make(map[uuid.UUID]*group)
	for _, ref := range refs {
		item := groups[ref.Scope.ID]
		if item == nil {
			item = &group{scope: ref.Scope}
			groups[ref.Scope.ID] = item
		}
		item.ids = append(item.ids, ref.RAGFlowDocumentID)
	}
	for _, item := range groups {
		if err := s.rag.UpdateDocumentMetadata(ctx, datasetID, item.ids, map[string]string{
			scopeMetadataKey: item.scope.ID.String(), "docs_hub_scope_type": item.scope.Type,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureChat(ctx context.Context, projectID uuid.UUID, datasetID string) (string, error) {
	chatID, err := s.repo.RAGFlowChatID(ctx, projectID)
	if err != nil {
		return "", err
	}
	if chatID != "" {
		return chatID, nil
	}
	name := "docs_hub_" + strings.ReplaceAll(projectID.String(), "-", "")
	chat, err := s.rag.FindChatByName(ctx, name)
	if err != nil {
		return "", err
	}
	if chat == nil {
		created, createErr := s.rag.CreateChat(ctx, name, []string{datasetID})
		if createErr != nil {
			return "", createErr
		}
		chat = &created
	} else if !contains(chat.DatasetIDs, datasetID) {
		if err = s.rag.UpdateChatDatasets(ctx, chat.ID, []string{datasetID}); err != nil {
			return "", err
		}
	}
	return s.repo.SaveRAGFlowChatID(ctx, projectID, chat.ID)
}

func (s *Service) authorize(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	actor, ok := contextx.ActorFrom(ctx)
	if !ok {
		return uuid.Nil, apperr.Unauthorized("Chưa xác thực")
	}
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return uuid.Nil, apperr.Unauthorized("Định danh người dùng không hợp lệ")
	}
	role, err := s.repo.MemberRole(ctx, projectID, actorID)
	if err != nil {
		return uuid.Nil, apperr.Database("Không thể kiểm tra quyền project").WithCause(err)
	}
	if role == "" {
		return uuid.Nil, apperr.Forbidden("Không có quyền truy cập project")
	}
	return actorID, nil
}

func intentFor(scope retrievaldomain.Scope) string {
	if scope.Mode == retrievaldomain.ScopeAll || len(scope.VersionIDs)+len(scope.ChangeRequestIDs) > 1 {
		return "evolution"
	}
	return "specific_revision"
}

func scopesFrom(refs []retrievaldomain.RevisionRef) []retrievaldomain.ResolvedScope {
	seen := make(map[uuid.UUID]struct{}, len(refs))
	out := make([]retrievaldomain.ResolvedScope, 0, len(refs))
	for _, ref := range refs {
		if _, ok := seen[ref.Scope.ID]; ok {
			continue
		}
		seen[ref.Scope.ID] = struct{}{}
		out = append(out, ref.Scope)
	}
	return out
}

func scopeConditions(scope retrievaldomain.Scope) []port.RAGMetadataCondition {
	if scope.Mode == retrievaldomain.ScopeAll {
		return nil
	}
	ids := append([]uuid.UUID(nil), scope.VersionIDs...)
	ids = append(ids, scope.ChangeRequestIDs...)
	out := make([]port.RAGMetadataCondition, len(ids))
	for i, id := range ids {
		out[i] = port.RAGMetadataCondition{Name: scopeMetadataKey, Operator: "is", Value: id.String()}
	}
	return out
}

func ragMessages(history []domain.Message, question string) []port.RAGChatMessage {
	const maxHistory = 10
	start := len(history) - maxHistory
	if start < 0 {
		start = 0
	}
	out := make([]port.RAGChatMessage, 0, len(history)-start+1)
	for _, message := range history[start:] {
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		out = append(out, port.RAGChatMessage{Role: message.Role, Content: message.Content})
	}
	return append(out, port.RAGChatMessage{Role: "user", Content: question})
}

func mapRAGCitations(
	projectID uuid.UUID, datasetID string, refs []retrievaldomain.RevisionRef, chunks []port.RAGChunk,
) []domain.Citation {
	allowed := make(map[string]retrievaldomain.RevisionRef, len(refs))
	for _, ref := range refs {
		allowed[ref.RAGFlowDocumentID] = ref
	}
	out := make([]domain.Citation, 0, len(chunks))
	seen := make(map[string]struct{}, len(chunks))
	for _, chunk := range chunks {
		ref, ok := allowed[chunk.DocumentID]
		if !ok || chunk.DatasetID != "" && chunk.DatasetID != datasetID {
			continue
		}
		if _, duplicate := seen[chunk.ID]; duplicate {
			continue
		}
		seen[chunk.ID] = struct{}{}
		out = append(out, domain.Citation{
			Key: fmt.Sprintf("S%d", len(out)+1), ChunkID: chunk.ID,
			DocumentID: ref.DocumentID, DocumentRevisionID: ref.RevisionID,
			DocumentTitle: ref.Title, DocumentName: ref.FileName,
			ScopeType: ref.Scope.Type, ScopeLabel: ref.Scope.Label, Excerpt: chunk.Content,
			SourceURL: fmt.Sprintf("/internal/api/v1/projects/%s/documents/%s/revisions/%s/view",
				projectID, ref.DocumentID, ref.RevisionID),
		})
	}
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
