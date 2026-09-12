// Package http cung cấp REST delivery cho tính năng Nhận diện & Phân tích
// Edge Case cho URD (URD v1.2 mục XI).
package http

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/urd/usecase"
)

type Handler struct{ svc *usecase.Service }

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// AnalysisResponse gói phân tích + danh sách edge case để FE dựng modal.
type AnalysisResponse struct {
	Analysis *domain.Analysis  `json:"analysis"`
	Cases    []domain.EdgeCase `json:"cases"`
}

// UploadImageResponse trả object key ảnh vừa lưu để đính vào ResolutionItem.
type UploadImageResponse struct {
	ImageObjectKey string `json:"image_object_key"`
}

// ResolutionItem là hướng giải quyết cho 1 edge case trong SubmitResolutionsRequest.
type ResolutionItem struct {
	CaseID         string `json:"case_id" binding:"required"`
	Resolution     string `json:"resolution" binding:"required"`
	ImageObjectKey string `json:"image_object_key"`
}

// SubmitResolutionsRequest là body xác nhận hướng giải quyết cho toàn bộ (hoặc
// một phần) edge case của 1 phân tích.
type SubmitResolutionsRequest struct {
	Items []ResolutionItem `json:"items" binding:"required,min=1,dive"`
}

// SummaryItem là 1 dòng tóm tắt độ hoàn thiện URD — cột "Hoàn thiện" trong
// bảng Quản lý dự án.
type SummaryItem struct {
	DocumentID    uuid.UUID `json:"document_id"`
	Status        string    `json:"status"`
	TotalCases    int       `json:"total_cases"`
	ResolvedCases int       `json:"resolved_cases"`
}

// Analyze godoc
// @Summary Nhờ AI liệt kê edge case chưa được đề cập trong tài liệu URD
// @Description Tài liệu phải đã được xác nhận doc_type=urd (PATCH .../documents/{document_id}/doc-type)
// @Description và có revision mới nhất đã ingest xong (status=ready).
// @Tags urd
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Success 201 {object} response.Envelope{data=AnalysisResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id}/urd/analyze [post]
func (h *Handler) Analyze(c *gin.Context) {
	pid, did, ok := documentIDs(c)
	if !ok {
		return
	}
	a, cases, err := h.svc.Analyze(c.Request.Context(), pid, did)
	if err != nil {
		fail(c, err)
		return
	}
	response.Created(c, AnalysisResponse{Analysis: a, Cases: cases})
}

// GetAnalysis godoc
// @Summary Xem chi tiết 1 phân tích edge case (mở lại modal đang dở)
// @Tags urd
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Param analysis_id path string true "Analysis ID" format(uuid)
// @Success 200 {object} response.Envelope{data=AnalysisResponse}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id}/urd/analyses/{analysis_id} [get]
func (h *Handler) GetAnalysis(c *gin.Context) {
	pid, did, aid, ok := analysisIDs(c)
	if !ok {
		return
	}
	a, cases, err := h.svc.Get(c.Request.Context(), pid, did, aid)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, AnalysisResponse{Analysis: a, Cases: cases})
}

// UploadCaseImage godoc
// @Summary Tải ảnh minh hoạ cho 1 edge case
// @Description Trả về image_object_key để đính vào item tương ứng khi gọi POST .../resolutions.
// @Tags urd
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Param analysis_id path string true "Analysis ID" format(uuid)
// @Param case_id path string true "Edge case ID" format(uuid)
// @Param file formData file true "Ảnh minh hoạ"
// @Success 200 {object} response.Envelope{data=UploadImageResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id}/urd/analyses/{analysis_id}/cases/{case_id}/image [post]
func (h *Handler) UploadCaseImage(c *gin.Context) {
	pid, did, aid, ok := analysisIDs(c)
	if !ok {
		return
	}
	cid, ok := pathID(c, "case_id")
	if !ok {
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		fail(c, apperr.BadRequest("Thiếu file ảnh"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		fail(c, apperr.BadRequest("Không thể đọc file ảnh"))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		fail(c, apperr.BadRequest("Không thể đọc file ảnh"))
		return
	}
	mediaType := http.DetectContentType(data)
	key, err := h.svc.UploadCaseImage(c.Request.Context(), pid, did, aid, cid, fh.Filename, mediaType, data)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, UploadImageResponse{ImageObjectKey: key})
}

// SubmitResolutions godoc
// @Summary Lưu hướng giải quyết cho các edge case; tạo phiên bản URD mới khi đã đủ
// @Description Khi resolved_cases đạt total_cases sau lời gọi này, hệ thống tự động
// @Description merge nội dung vào cuối file .docx gốc và tạo revision mới.
// @Tags urd
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Param analysis_id path string true "Analysis ID" format(uuid)
// @Param body body SubmitResolutionsRequest true "Hướng giải quyết từng case"
// @Success 200 {object} response.Envelope{data=domain.Analysis}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id}/urd/analyses/{analysis_id}/resolutions [post]
func (h *Handler) SubmitResolutions(c *gin.Context) {
	pid, did, aid, ok := analysisIDs(c)
	if !ok {
		return
	}
	var req SubmitResolutionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, apperr.BadRequest("Dữ liệu không hợp lệ"))
		return
	}
	items := make([]usecase.ResolutionInput, len(req.Items))
	for i, item := range req.Items {
		caseID, err := uuid.Parse(item.CaseID)
		if err != nil {
			fail(c, apperr.BadRequest("case_id không hợp lệ"))
			return
		}
		items[i] = usecase.ResolutionInput{
			CaseID: caseID, Resolution: item.Resolution, ImageObjectKey: item.ImageObjectKey,
		}
	}
	a, err := h.svc.SubmitResolutions(c.Request.Context(), pid, did, aid, items)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, a)
}

// ListSummaries godoc
// @Summary Tóm tắt độ hoàn thiện URD của toàn bộ tài liệu trong project
// @Description Dùng hiển thị cột "Hoàn thiện" trong bảng Quản lý dự án — chỉ tài
// @Description liệu nào đã từng phân tích mới xuất hiện trong kết quả.
// @Tags urd
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Success 200 {object} response.Envelope{data=[]SummaryItem}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/urd-summary [get]
func (h *Handler) ListSummaries(c *gin.Context) {
	pid, ok := pathID(c, "id")
	if !ok {
		return
	}
	summaries, err := h.svc.ListSummaries(c.Request.Context(), pid)
	if err != nil {
		fail(c, err)
		return
	}
	items := make([]SummaryItem, 0, len(summaries))
	for documentID, a := range summaries {
		items = append(items, SummaryItem{
			DocumentID: documentID, Status: a.Status,
			TotalCases: a.TotalCases, ResolvedCases: a.ResolvedCases,
		})
	}
	response.OK(c, items)
}

func documentIDs(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	pid, ok := pathID(c, "id")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	did, ok := pathID(c, "document_id")
	return pid, did, ok
}

func analysisIDs(c *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	pid, did, ok := documentIDs(c)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	aid, ok := pathID(c, "analysis_id")
	return pid, did, aid, ok
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
