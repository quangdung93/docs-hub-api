// Package http cung cấp REST delivery cho module project.
package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/project/usecase"
)

type Handler struct{ service *usecase.Service }

// CreateResponse là project public; remote dataset ID luôn bị ẩn bởi JSON contract.
type CreateResponse = domain.Project

func New(service *usecase.Service) *Handler { return &Handler{service: service} }

// CreateRequest là body tạo project và dataset RAGFlow tương ứng.
type CreateRequest struct {
	Code        string `json:"code" binding:"required,min=2,max=64"`
	Name        string `json:"name" binding:"required,max=255"`
	Description string `json:"description"`
}

// CreateVersionRequest là body tạo draft version trong project.
type CreateVersionRequest struct {
	Label string `json:"label" binding:"required,max=100"`
}

// Create godoc
// @Summary Tạo project và dataset RAGFlow
// @Description Tạo project local, owner membership và dataset riêng trên RAGFlow; thất bại nếu không tạo được cả hai phía.
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CreateRequest true "Thông tin project"
// @Success 201 {object} response.Envelope{data=CreateResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Failure 504 {object} response.Envelope
// @Router /internal/api/v1/projects [post]
func (h *Handler) Create(c *gin.Context) {
	var request CreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(apperr.BadRequest("Dữ liệu tạo project không hợp lệ"))
		return
	}
	project, err := h.service.Create(c.Request.Context(), usecase.CreateInput{
		Code: request.Code, Name: request.Name, Description: request.Description,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.Created(c, project)
}

// CreateVersion godoc
// @Summary Tạo draft version
// @Description Owner hoặc editor tạo version mới. Version chỉ lưu local và dùng chung dataset RAGFlow của project.
// @Tags project-versions
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param project_id path string true "Project ID" format(uuid)
// @Param body body CreateVersionRequest true "Thông tin version"
// @Success 201 {object} response.Envelope{data=domain.ProjectVersion}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /internal/api/v1/projects/{project_id}/versions [post]
func (h *Handler) CreateVersion(c *gin.Context) {
	projectID, ok := projectIDFromPath(c)
	if !ok {
		return
	}
	var request CreateVersionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(apperr.BadRequest("Dữ liệu tạo version không hợp lệ"))
		return
	}
	version, err := h.service.CreateVersion(c.Request.Context(), usecase.CreateVersionInput{
		ProjectID: projectID, Label: request.Label,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.Created(c, version)
}

// ListVersions godoc
// @Summary Timeline version của project
// @Description Thành viên project xem các version theo sequence_no giảm dần.
// @Tags project-versions
// @Security BearerAuth
// @Produce json
// @Param project_id path string true "Project ID" format(uuid)
// @Param page query int false "Trang" default(1)
// @Param limit query int false "Số bản ghi/trang" default(20)
// @Success 200 {object} response.Envelope{data=[]domain.ProjectVersion}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{project_id}/versions [get]
func (h *Handler) ListVersions(c *gin.Context) {
	projectID, ok := projectIDFromPath(c)
	if !ok {
		return
	}
	var page pagination.Query
	if err := c.ShouldBindQuery(&page); err != nil {
		_ = c.Error(apperr.BadRequest("Phân trang không hợp lệ"))
		return
	}
	versions, meta, err := h.service.ListVersions(c.Request.Context(), projectID, page)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OKPaged(c, versions, meta)
}

func projectIDFromPath(c *gin.Context) (uuid.UUID, bool) {
	projectID, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		_ = c.Error(apperr.BadRequest("Project ID không hợp lệ"))
		return uuid.Nil, false
	}
	return projectID, true
}
