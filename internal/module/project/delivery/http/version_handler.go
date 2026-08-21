package http

import (
	"github.com/gin-gonic/gin"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/project/usecase"
)

type CreateVersionRequest struct {
	Label string `json:"label" binding:"required,max=100"`
}

type ProjectVersionResponse = domain.ProjectVersion

// CreateVersion godoc
// @Summary Tạo draft version
// @Description Owner hoặc editor tạo version mới; version dùng chung dataset RAGFlow của project.
// @Tags project-versions
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param body body CreateVersionRequest true "Thông tin version"
// @Success 201 {object} response.Envelope{data=ProjectVersionResponse}
// @Router /internal/api/v1/projects/{id}/versions [post]
func (h *Handler) CreateVersion(c *gin.Context) {
	projectID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req CreateVersionRequest
	if !bindBody(c, &req) {
		return
	}
	version, err := h.svc.CreateVersion(c.Request.Context(), usecase.CreateVersionInput{
		ProjectID: projectID,
		Label:     req.Label,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.Created(c, version)
}

// ListVersions godoc
// @Summary Timeline version của project
// @Description Thành viên active xem các version theo sequence_no giảm dần.
// @Tags project-versions
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param page query int false "Trang" default(1)
// @Param limit query int false "Số bản ghi/trang" default(20)
// @Success 200 {object} response.Envelope{data=[]ProjectVersionResponse}
// @Router /internal/api/v1/projects/{id}/versions [get]
func (h *Handler) ListVersions(c *gin.Context) {
	projectID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var page pagination.Query
	if err := c.ShouldBindQuery(&page); err != nil {
		_ = c.Error(apperr.BadRequest("Phân trang không hợp lệ"))
		return
	}
	versions, meta, err := h.svc.ListVersions(c.Request.Context(), projectID, page)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OKPaged(c, versions, meta)
}
