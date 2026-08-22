// Package usecase điều phối retrieval có project ACL và version scope.
package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

const maxQuestionLength = 8000

type Service struct {
	repo      domain.Repository
	rag       Retriever
	bypassACL bool
}

type Retriever interface {
	Retrieve(ctx context.Context, input port.RAGRetrievalRequest) (port.RAGRetrievalResult, error)
}

func New(repo domain.Repository, rag Retriever, bypassACL bool) *Service {
	return &Service{repo: repo, rag: rag, bypassACL: bypassACL}
}

type Input struct {
	ProjectID              uuid.UUID
	Scope                  domain.Scope
	Query                  string
	PageSize               int
	SimilarityThreshold    float64
	VectorSimilarityWeight float64
	Keyword                bool
}

type Citation struct {
	Key                string    `json:"key"`
	ChunkID            string    `json:"chunk_id"`
	DocumentID         uuid.UUID `json:"document_id"`
	DocumentRevisionID uuid.UUID `json:"document_revision_id"`
	DocumentTitle      string    `json:"-"`
	DocumentName       string    `json:"document_name"`
	ScopeType          string    `json:"scope_type"`
	ScopeLabel         string    `json:"scope_label"`
	LineStart          *int      `json:"line_start"`
	LineEnd            *int      `json:"line_end"`
	PageStart          *int      `json:"page_start"`
	PageEnd            *int      `json:"page_end"`
	Excerpt            string    `json:"excerpt"`
	SourceURL          string    `json:"source_url"`
	Score              float64   `json:"score"`
	VectorScore        float64   `json:"vector_score"`
	TermScore          float64   `json:"term_score"`
}

type Result struct {
	Query         string                 `json:"query"`
	ResolvedScope []domain.ResolvedScope `json:"resolved_scope"`
	Citations     []Citation             `json:"citations"`
	Total         int                    `json:"total"`
}

func (s *Service) Retrieve(ctx context.Context, in Input) (*Result, error) {
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" || len(in.Query) > maxQuestionLength {
		return nil, apperr.BadRequest("Câu hỏi rỗng hoặc vượt quá 8000 ký tự")
	}
	if !in.Scope.Valid() {
		return nil, apperr.BadRequest("Scope chỉ hỗ trợ versions, change_requests hoặc all với danh sách ID phù hợp")
	}
	if err := s.authorize(ctx, in.ProjectID); err != nil {
		return nil, err
	}
	resolved, err := s.repo.ResolveScope(ctx, in.ProjectID, in.Scope)
	if err != nil {
		return nil, apperr.Database("Không thể kiểm tra scope").WithCause(err)
	}
	expected := len(in.Scope.VersionIDs) + len(in.Scope.ChangeRequestIDs)
	if in.Scope.Mode != domain.ScopeAll && len(resolved) != expected {
		return nil, apperr.NotFound("NOT_FOUND", "Không tìm thấy version hoặc change request")
	}
	datasetID, err := s.repo.DatasetID(ctx, in.ProjectID)
	if err != nil {
		return nil, apperr.Database("Không thể đọc RAGFlow dataset mapping").WithCause(err)
	}
	if datasetID == "" {
		return nil, apperr.External("Project chưa được đồng bộ sang RAGFlow")
	}
	refs, err := s.repo.RevisionRefs(ctx, in.ProjectID, in.Scope)
	if err != nil {
		return nil, apperr.Database("Không thể đọc revision mapping").WithCause(err)
	}
	if len(refs) == 0 {
		return &Result{Query: in.Query, ResolvedScope: resolved, Citations: []Citation{}}, nil
	}
	if in.Scope.Mode == domain.ScopeAll {
		resolved = scopesFrom(refs)
	}
	citations := make([]Citation, 0)
	for _, group := range groupByScope(resolved, refs) {
		groupCitations, retrieveErr := s.retrieveGroup(ctx, in, datasetID, group)
		if retrieveErr != nil {
			return nil, retrieveErr
		}
		citations = append(citations, groupCitations...)
	}
	for i := range citations {
		citations[i].Key = fmt.Sprintf("S%d", i+1)
	}
	return &Result{Query: in.Query, ResolvedScope: resolved, Citations: citations, Total: len(citations)}, nil
}

func (s *Service) authorize(ctx context.Context, projectID uuid.UUID) error {
	actor, ok := contextx.ActorFrom(ctx)
	if !ok {
		return apperr.Unauthorized("Chưa xác thực")
	}
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return apperr.Unauthorized("Định danh người dùng không hợp lệ")
	}
	if s.bypassACL {
		return nil
	}
	role, err := s.repo.MemberRole(ctx, projectID, actorID)
	if err != nil {
		return apperr.Database("Không thể kiểm tra quyền project").WithCause(err)
	}
	if role == "" {
		return apperr.Forbidden("Không có quyền truy cập project")
	}
	return nil
}

func (s *Service) retrieveGroup(
	ctx context.Context, in Input, datasetID string, refs []domain.RevisionRef,
) ([]Citation, error) {
	remoteIDs := make([]string, len(refs))
	allowed := make(map[string]domain.RevisionRef, len(refs))
	for i, ref := range refs {
		remoteIDs[i] = ref.RAGFlowDocumentID
		allowed[ref.RAGFlowDocumentID] = ref
	}
	pageSize, threshold, weight := retrievalOptions(in)
	remote, err := s.rag.Retrieve(ctx, port.RAGRetrievalRequest{
		Question: in.Query, DatasetIDs: []string{datasetID}, DocumentIDs: remoteIDs,
		Page: 1, PageSize: pageSize, SimilarityThreshold: threshold,
		VectorSimilarityWeight: weight, Keyword: in.Keyword,
	})
	if err != nil {
		return nil, apperr.External("RAGFlow retrieval không khả dụng").WithCause(fmt.Errorf("retrieve: %w", err))
	}
	citations := make([]Citation, 0, len(remote.Chunks))
	for _, chunk := range remote.Chunks {
		ref, permitted := allowed[chunk.DocumentID]
		if !permitted || chunk.DatasetID != "" && chunk.DatasetID != datasetID {
			continue
		}
		citations = append(citations, Citation{
			ChunkID: chunk.ID, DocumentID: ref.DocumentID, DocumentRevisionID: ref.RevisionID,
			DocumentTitle: ref.Title, DocumentName: ref.FileName,
			ScopeType: ref.Scope.Type, ScopeLabel: ref.Scope.Label,
			Excerpt: chunk.Content, SourceURL: sourceURL(in.ProjectID, ref), Score: chunk.Similarity,
			VectorScore: chunk.VectorSimilarity, TermScore: chunk.TermSimilarity,
		})
	}
	return citations, nil
}

func retrievalOptions(in Input) (int, float64, float64) {
	pageSize := in.PageSize
	if pageSize < 1 {
		pageSize = 10
	} else if pageSize > 50 {
		pageSize = 50
	}
	threshold := in.SimilarityThreshold
	if threshold <= 0 {
		threshold = 0.2
	}
	weight := in.VectorSimilarityWeight
	if weight <= 0 || weight > 1 {
		weight = 0.3
	}
	return pageSize, threshold, weight
}

func sourceURL(projectID uuid.UUID, ref domain.RevisionRef) string {
	return fmt.Sprintf("/internal/api/v1/projects/%s/documents/%s/revisions/%s/view",
		projectID, ref.DocumentID, ref.RevisionID)
}

func scopesFrom(refs []domain.RevisionRef) []domain.ResolvedScope {
	seen := make(map[uuid.UUID]struct{}, len(refs))
	resolved := make([]domain.ResolvedScope, 0, len(refs))
	for _, ref := range refs {
		if _, exists := seen[ref.Scope.ID]; exists {
			continue
		}
		seen[ref.Scope.ID] = struct{}{}
		resolved = append(resolved, ref.Scope)
	}
	return resolved
}

func groupByScope(resolved []domain.ResolvedScope, refs []domain.RevisionRef) [][]domain.RevisionRef {
	byID := make(map[uuid.UUID][]domain.RevisionRef, len(resolved))
	for _, ref := range refs {
		byID[ref.Scope.ID] = append(byID[ref.Scope.ID], ref)
	}
	groups := make([][]domain.RevisionRef, 0, len(resolved))
	for _, scope := range resolved {
		if group := byID[scope.ID]; len(group) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}
