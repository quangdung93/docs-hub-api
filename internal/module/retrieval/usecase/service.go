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
	Question               string
	PageSize               int
	SimilarityThreshold    float64
	VectorSimilarityWeight float64
	Keyword                bool
}

type Citation struct {
	ChunkID     string    `json:"chunk_id"`
	DocumentID  uuid.UUID `json:"document_id"`
	RevisionID  uuid.UUID `json:"revision_id"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	Score       float64   `json:"score"`
	VectorScore float64   `json:"vector_score"`
	TermScore   float64   `json:"term_score"`
}

type Result struct {
	Question  string     `json:"question"`
	Citations []Citation `json:"citations"`
	Total     int        `json:"total"`
}

func (s *Service) Retrieve(ctx context.Context, in Input) (*Result, error) {
	in.Question = strings.TrimSpace(in.Question)
	if in.Question == "" || len(in.Question) > maxQuestionLength {
		return nil, apperr.BadRequest("Câu hỏi rỗng hoặc vượt quá 8000 ký tự")
	}
	if !in.Scope.Valid() {
		return nil, apperr.BadRequest("Phải truyền đúng một project_version_id hoặc change_request_id")
	}
	actor, ok := contextx.ActorFrom(ctx)
	if !ok {
		return nil, apperr.Unauthorized("Chưa xác thực")
	}
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return nil, apperr.Unauthorized("Định danh người dùng không hợp lệ")
	}
	if !s.bypassACL {
		role, roleErr := s.repo.MemberRole(ctx, in.ProjectID, actorID)
		if roleErr != nil {
			return nil, apperr.Database("Không thể kiểm tra quyền project").WithCause(roleErr)
		}
		if role == "" {
			return nil, apperr.Forbidden("Không có quyền truy cập project")
		}
	}
	exists, err := s.repo.ScopeExists(ctx, in.ProjectID, in.Scope)
	if err != nil {
		return nil, apperr.Database("Không thể kiểm tra scope").WithCause(err)
	}
	if !exists {
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
		return nil, apperr.BadRequest("Scope chưa có revision sẵn sàng trên RAGFlow")
	}
	remoteIDs := make([]string, len(refs))
	allowed := make(map[string]domain.RevisionRef, len(refs))
	for i, ref := range refs {
		remoteIDs[i] = ref.RAGFlowDocumentID
		allowed[ref.RAGFlowDocumentID] = ref
	}
	pageSize := in.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 50 {
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
	remote, err := s.rag.Retrieve(ctx, port.RAGRetrievalRequest{
		Question: in.Question, DatasetIDs: []string{datasetID}, DocumentIDs: remoteIDs,
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
			ChunkID: chunk.ID, DocumentID: ref.DocumentID, RevisionID: ref.RevisionID,
			Title: ref.Title, Excerpt: chunk.Content, Score: chunk.Similarity,
			VectorScore: chunk.VectorSimilarity, TermScore: chunk.TermSimilarity,
		})
	}
	return &Result{Question: in.Question, Citations: citations, Total: len(citations)}, nil
}
