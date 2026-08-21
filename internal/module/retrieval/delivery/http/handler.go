package http

import (
	"errors"

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
	Question               string  `json:"question" binding:"required"`
	ProjectVersionID       string  `json:"project_version_id"`
	ChangeRequestID        string  `json:"change_request_id"`
	PageSize               int     `json:"page_size"`
	SimilarityThreshold    float64 `json:"similarity_threshold"`
	VectorSimilarityWeight float64 `json:"vector_similarity_weight"`
	Keyword                bool    `json:"keyword"`
}

// Retrieve godoc
// @Summary Truy hồi tài liệu theo project version hoặc change request
// @Tags retrieval
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param project_id path string true "Project ID" format(uuid)
// @Param body body Request true "Câu hỏi và scope"
// @Success 200 {object} response.Envelope{data=usecase.Result}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 504 {object} response.Envelope
// @Router /internal/api/v1/projects/{project_id}/retrieval [post]
func (h *Handler) Retrieve(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		_ = c.Error(apperr.BadRequest("Project ID không hợp lệ"))
		return
	}
	var request Request
	if err = c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(apperr.BadRequest("Dữ liệu retrieval không hợp lệ"))
		return
	}
	scope, err := parseScope(request.ProjectVersionID, request.ChangeRequestID)
	if err != nil {
		_ = c.Error(apperr.BadRequest(err.Error()))
		return
	}
	result, err := h.service.Retrieve(c.Request.Context(), usecase.Input{
		ProjectID: projectID, Scope: scope, Question: request.Question, PageSize: request.PageSize,
		SimilarityThreshold:    request.SimilarityThreshold,
		VectorSimilarityWeight: request.VectorSimilarityWeight, Keyword: request.Keyword,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, result)
}

func parseScope(versionID, changeID string) (domain.Scope, error) {
	var scope domain.Scope
	if versionID != "" {
		id, err := uuid.Parse(versionID)
		if err != nil {
			return scope, errors.New("Project version ID không hợp lệ")
		}
		scope.VersionID = &id
	}
	if changeID != "" {
		id, err := uuid.Parse(changeID)
		if err != nil {
			return scope, errors.New("Change request ID không hợp lệ")
		}
		scope.ChangeRequestID = &id
	}
	if !scope.Valid() {
		return scope, errors.New("Phải truyền đúng một project_version_id hoặc change_request_id")
	}
	return scope, nil
}
