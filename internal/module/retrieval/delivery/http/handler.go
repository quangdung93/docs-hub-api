package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/retrieval/usecase"
)

type Handler struct{ service *usecase.Service }

func New(service *usecase.Service) *Handler { return &Handler{service: service} }

type Request struct {
	Query                  string       `json:"query" binding:"required"`
	Scope                  ScopeRequest `json:"scope" binding:"required"`
	PageSize               int          `json:"page_size"`
	SimilarityThreshold    float64      `json:"similarity_threshold"`
	VectorSimilarityWeight float64      `json:"vector_similarity_weight"`
	Keyword                bool         `json:"keyword"`
}

type ScopeRequest struct {
	Mode             string   `json:"mode" binding:"required"`
	VersionIDs       []string `json:"version_ids"`
	ChangeRequestIDs []string `json:"change_request_ids"`
}

// Retrieve godoc
// @Summary Truy hồi tài liệu theo project version hoặc change request
// @Tags retrieval
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param body body Request true "Câu hỏi và scope"
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 504 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/search [post]
func (h *Handler) Retrieve(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(apperr.BadRequest("Project ID không hợp lệ"))
		return
	}
	var request Request
	if err = c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(apperr.BadRequest("Dữ liệu retrieval không hợp lệ"))
		return
	}
	scope, err := ParseScope(request.Scope)
	if err != nil {
		_ = c.Error(apperr.BadRequest(err.Error()))
		return
	}
	result, err := h.service.Retrieve(c.Request.Context(), usecase.Input{
		ProjectID: projectID, Scope: scope, Query: request.Query, PageSize: request.PageSize,
		SimilarityThreshold:    request.SimilarityThreshold,
		VectorSimilarityWeight: request.VectorSimilarityWeight, Keyword: request.Keyword,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, result)
}

func ParseScope(input ScopeRequest) (domain.Scope, error) {
	scope := domain.Scope{Mode: input.Mode}
	for _, raw := range input.VersionIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return scope, apperr.BadRequest("Project version ID không hợp lệ")
		}
		scope.VersionIDs = append(scope.VersionIDs, id)
	}
	for _, raw := range input.ChangeRequestIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return scope, apperr.BadRequest("Change request ID không hợp lệ")
		}
		scope.ChangeRequestIDs = append(scope.ChangeRequestIDs, id)
	}
	if !scope.Valid() {
		return scope, apperr.BadRequest("Scope chỉ hỗ trợ versions, change_requests hoặc all với danh sách ID phù hợp")
	}
	return scope, nil
}
