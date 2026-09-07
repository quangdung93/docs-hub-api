// Package http cung cấp REST delivery cho xuất báo cáo dự án (SRS v1.1 mục IX/X).
package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/module/report/usecase"
)

type Handler struct{ svc *usecase.Service }

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// GenerateRequest là body xuất báo cáo. ProjectVersionID/ChangeRequestID tùy
// chọn — chỉ được điền đúng 1 trong 2, để trống cả hai = lấy toàn bộ tài liệu
// mới nhất của project.
type GenerateRequest struct {
	ReportType       string `json:"report_type" binding:"required" example:"uat" enums:"uat,planning,testcase"`
	Format           string `json:"format" example:"xlsx" enums:"xlsx,pdf"`
	ProjectVersionID string `json:"project_version_id"`
	ChangeRequestID  string `json:"change_request_id"`
}

// Generate godoc
// @Summary Xuất báo cáo dự án (UAT Report / Project Planning / Testcase)
// @Description Nhờ RAGFlow tổng hợp nội dung tài liệu dự án thành báo cáo theo loại
// @Description đã chọn (uat, planning, testcase), giới hạn theo project_version_id
// @Description hoặc change_request_id nếu có (để trống cả hai = toàn bộ tài liệu mới
// @Description nhất). Chỉ Editor trở lên được xuất.
// @Tags reports
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param body body GenerateRequest true "Loại báo cáo, định dạng và phạm vi tùy chọn"
// @Success 201 {object} usecase.GenerateResult
// @Router /projects/{id}/reports [post]
func (h *Handler) Generate(c *gin.Context) {
	pid, ok := pathID(c, "id")
	if !ok {
		return
	}
	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, apperr.BadRequest("Body không hợp lệ"))
		return
	}
	versionID, ok := optionalIDPtr(req.ProjectVersionID)
	if !ok {
		fail(c, apperr.BadRequest("project_version_id không hợp lệ"))
		return
	}
	changeRequestID, ok := optionalIDPtr(req.ChangeRequestID)
	if !ok {
		fail(c, apperr.BadRequest("change_request_id không hợp lệ"))
		return
	}
	result, err := h.svc.Generate(c.Request.Context(), usecase.GenerateInput{
		ProjectID: pid, ReportType: req.ReportType, Format: req.Format,
		VersionID: versionID, ChangeRequestID: changeRequestID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	response.Created(c, result)
}

// History godoc
// @Summary Lịch sử các lần xuất báo cáo của dự án
// @Tags reports
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} usecase.HistoryItem
// @Router /projects/{id}/reports/history [get]
func (h *Handler) History(c *gin.Context) {
	pid, ok := pathID(c, "id")
	if !ok {
		return
	}
	var p pagination.Query
	if err := c.ShouldBindQuery(&p); err != nil {
		fail(c, apperr.BadRequest("Phân trang không hợp lệ"))
		return
	}
	items, meta, err := h.svc.ListHistory(c.Request.Context(), pid, p)
	if err != nil {
		fail(c, err)
		return
	}
	response.OKPaged(c, items, meta)
}

// optionalIDPtr parse UUID tùy chọn — rỗng trả (nil, true); sai định dạng trả
// (nil, false) để caller tự báo lỗi phù hợp field.
func optionalIDPtr(raw string) (*uuid.UUID, bool) {
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, false
	}
	return &id, true
}

func pathID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		fail(c, apperr.BadRequest("ID không hợp lệ"))
		return uuid.Nil, false
	}
	return id, true
}

func fail(c *gin.Context, err error) { _ = c.Error(err) }
