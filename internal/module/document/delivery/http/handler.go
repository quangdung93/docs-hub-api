// Package http cung cấp REST delivery cho upload và quản lý tài liệu.
package http

import (
	"bufio"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/document/usecase"
)

type Handler struct{ svc *usecase.Service }

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// PresignRequest là body tạo phiên upload trực tiếp qua S3/MinIO.
type PresignRequest struct {
	DocumentID       string `json:"document_id"`
	Title            string `json:"title" binding:"max=255"`
	Description      string `json:"description"`
	FileName         string `json:"file_name" binding:"required"`
	MediaType        string `json:"media_type" binding:"required"`
	SizeBytes        int64  `json:"size_bytes" binding:"required,min=1"`
	SHA256           string `json:"sha256" binding:"required"`
	ProjectVersionID string `json:"project_version_id"`
	ChangeRequestID  string `json:"change_request_id"`
}

// UpdateRequest là body cập nhật metadata document với optimistic lock.
type UpdateRequest struct {
	Title       string `json:"title" binding:"required,max=255"`
	Description string `json:"description"`
	Version     int    `json:"version" binding:"required,min=1"`
}

// UploadResponse mô tả document và revision vừa được đưa vào hàng đợi ingestion.
type UploadResponse struct {
	Document *domain.Document `json:"document"`
	Revision *domain.Revision `json:"revision"`
}

// DocumentDetailResponse trả document cùng lịch sử revision.
type DocumentDetailResponse struct {
	Document  *domain.Document  `json:"document"`
	Revisions []domain.Revision `json:"revisions"`
}

// RetryResponse là trạng thái sau khi enqueue lại ingestion.
type RetryResponse struct {
	Status string `json:"status" example:"queued"`
}

// Upload godoc
// @Summary Upload tài liệu mới
// @Description Upload multipart trực tiếp. Phải truyền đúng một project_version_id hoặc change_request_id.
// @Tags documents
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param file formData file true "File TXT, MD, CSV, PDF text-layer, DOCX hoặc XLSX"
// @Param title formData string true "Tiêu đề document mới"
// @Param description formData string false "Mô tả"
// @Param document_id formData string false "Document ID nếu thêm revision" format(uuid)
// @Param project_version_id formData string false "Project version ID" format(uuid)
// @Param change_request_id formData string false "Change request ID" format(uuid)
// @Param size_bytes formData integer false "Kích thước khai báo để đối chiếu"
// @Param sha256 formData string true "SHA-256 dạng hex, 64 ký tự"
// @Success 202 {object} response.Envelope{data=UploadResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/uploads [post]
func (h *Handler) Upload(c *gin.Context) {
	h.upload(c, uuid.Nil)
}

func (h *Handler) upload(c *gin.Context, pathDocumentID uuid.UUID) {
	pid, ok := pathID(c, "id")
	if !ok {
		return
	}
	did := pathDocumentID
	if did == uuid.Nil {
		var valid bool
		did, valid = optionalID(c.PostForm("document_id"))
		if !valid {
			fail(c, apperr.BadRequest("Document ID không hợp lệ"))
			return
		}
	}
	scope, ok := formScope(c)
	if !ok {
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		fail(c, apperr.BadRequest("Thiếu file upload"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		fail(c, apperr.BadRequest("Không thể đọc file upload"))
		return
	}
	defer f.Close()
	size := fh.Size
	declared, _ := strconv.ParseInt(c.PostForm("size_bytes"), 10, 64)
	if declared > 0 && declared != size {
		fail(c, apperr.BadRequest("Kích thước khai báo không khớp"))
		return
	}
	reader := bufio.NewReader(f)
	head, _ := reader.Peek(512)
	mediaType := http.DetectContentType(head)
	if mediaType == "text/plain; charset=utf-8" {
		mediaType = "text/plain"
	}
	mediaType = usecase.MediaTypeForUpload(fh.Filename, mediaType)
	input := usecase.UploadInput{
		ProjectID: pid, DocumentID: did, Scope: scope,
		Title: c.PostForm("title"), Description: c.PostForm("description"),
		FileName: fh.Filename, MediaType: mediaType, SizeBytes: size,
		SHA256: c.PostForm("sha256"), Reader: reader,
	}
	d, r, err := h.svc.Upload(c.Request.Context(), input)
	if err != nil {
		fail(c, err)
		return
	}
	response.Accepted(c, gin.H{"document": d, "revision": r})
}

// Presign godoc
// @Summary Tạo presigned upload
// @Description Chỉ khả dụng khi storage.driver=minio; local filesystem trả HTTP 400 và dùng multipart upload.
// @Tags documents
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param body body PresignRequest true "Thông tin file và scope"
// @Success 200 {object} response.Envelope{data=usecase.PresignResult}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/uploads/presign [post]
func (h *Handler) Presign(c *gin.Context) {
	pid, ok := pathID(c, "id")
	if !ok {
		return
	}
	var req PresignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, apperr.BadRequest("Dữ liệu không hợp lệ"))
		return
	}
	did, valid := optionalID(req.DocumentID)
	if !valid {
		fail(c, apperr.BadRequest("Document ID không hợp lệ"))
		return
	}
	scope, ok := scopeIDs(c, req.ProjectVersionID, req.ChangeRequestID)
	if !ok {
		return
	}
	input := usecase.PresignInput{
		ProjectID: pid, DocumentID: did, Scope: scope,
		Title: req.Title, Description: req.Description, FileName: req.FileName,
		MediaType: req.MediaType, SizeBytes: req.SizeBytes, SHA256: req.SHA256,
	}
	out, err := h.svc.Presign(c.Request.Context(), input)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, out)
}

// Complete godoc
// @Summary Hoàn tất presigned upload
// @Tags documents
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param upload_id path string true "Upload session ID" format(uuid)
// @Success 202 {object} response.Envelope{data=UploadResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/uploads/{upload_id}/complete [post]
func (h *Handler) Complete(c *gin.Context) {
	pid, ok := pathID(c, "id")
	if !ok {
		return
	}
	uid, ok := pathID(c, "upload_id")
	if !ok {
		return
	}
	d, r, err := h.svc.Complete(c.Request.Context(), pid, uid)
	if err != nil {
		fail(c, err)
		return
	}
	response.Accepted(c, gin.H{"document": d, "revision": r})
}

// List godoc
// @Summary Danh sách tài liệu trong project
// @Tags documents
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param page query int false "Trang" default(1)
// @Param limit query int false "Số bản ghi/trang" default(20)
// @Param q query string false "Tìm theo tiêu đề"
// @Param status query string false "Trạng thái revision"
// @Param type query string false "MIME type"
// @Param version_id query string false "Project version ID" format(uuid)
// @Param change_request_id query string false "Change request ID" format(uuid)
// @Success 200 {object} response.Envelope{data=[]domain.Document}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents [get]
func (h *Handler) List(c *gin.Context) {
	pid, ok := pathID(c, "id")
	if !ok {
		return
	}
	var p pagination.Query
	if err := c.ShouldBindQuery(&p); err != nil {
		fail(c, apperr.BadRequest("Phân trang không hợp lệ"))
		return
	}
	f := domain.Filter{Query: c.Query("q"), Status: c.Query("status"), MediaType: c.Query("type")}
	f.VersionID, _ = optionalIDPtr(c.Query("version_id"))
	f.ChangeRequestID, _ = optionalIDPtr(c.Query("change_request_id"))
	items, meta, err := h.svc.List(c.Request.Context(), pid, f, p)
	if err != nil {
		fail(c, err)
		return
	}
	response.OKPaged(c, items, meta)
}

// Detail godoc
// @Summary Chi tiết tài liệu và các revision
// @Tags documents
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Success 200 {object} response.Envelope{data=DocumentDetailResponse}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id} [get]
func (h *Handler) Detail(c *gin.Context) {
	pid, did, ok := documentIDs(c)
	if !ok {
		return
	}
	d, rs, err := h.svc.Detail(c.Request.Context(), pid, did)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, gin.H{"document": d, "revisions": rs})
}

// Update godoc
// @Summary Cập nhật metadata tài liệu
// @Tags documents
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Param body body UpdateRequest true "Metadata và version hiện tại"
// @Success 200 {object} response.Envelope{data=domain.Document}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id} [patch]
func (h *Handler) Update(c *gin.Context) {
	pid, did, ok := documentIDs(c)
	if !ok {
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, apperr.BadRequest("Dữ liệu không hợp lệ"))
		return
	}
	d, err := h.svc.Update(c.Request.Context(), pid, did, req.Title, req.Description, req.Version)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, d)
}

// RevisionUpload godoc
// @Summary Upload revision mới cho document
// @Description Upload multipart trực tiếp vào document có sẵn. Phải truyền đúng một version hoặc change request.
// @Tags documents
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Param file formData file true "File tài liệu"
// @Param project_version_id formData string false "Project version ID" format(uuid)
// @Param change_request_id formData string false "Change request ID" format(uuid)
// @Param size_bytes formData integer false "Kích thước khai báo"
// @Param sha256 formData string true "SHA-256 dạng hex, 64 ký tự"
// @Success 202 {object} response.Envelope{data=UploadResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id}/revisions [post]
func (h *Handler) RevisionUpload(c *gin.Context) {
	_, did, ok := documentIDs(c)
	if !ok {
		return
	}
	h.upload(c, did)
}

// Status godoc
// @Summary Xem trạng thái ingestion của revision
// @Tags documents
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Param revision_id path string true "Revision ID" format(uuid)
// @Success 200 {object} response.Envelope{data=domain.Revision}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id}/revisions/{revision_id}/status [get]
func (h *Handler) Status(c *gin.Context) {
	pid, did, rid, ok := revisionIDs(c)
	if !ok {
		return
	}
	r, err := h.svc.Status(c.Request.Context(), pid, did, rid)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, r)
}

// Retry godoc
// @Summary Chạy lại ingestion cho revision thất bại
// @Tags documents
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Param revision_id path string true "Revision ID" format(uuid)
// @Success 202 {object} response.Envelope{data=RetryResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id}/revisions/{revision_id}/retry [post]
func (h *Handler) Retry(c *gin.Context) {
	pid, did, rid, ok := revisionIDs(c)
	if !ok {
		return
	}
	if err := h.svc.Retry(c.Request.Context(), pid, did, rid); err != nil {
		fail(c, err)
		return
	}
	response.Accepted(c, gin.H{"status": "queued"})
}

// Download godoc
// @Summary Tải file revision gốc
// @Tags documents
// @Security BearerAuth
// @Produce application/octet-stream
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Param revision_id path string true "Revision ID" format(uuid)
// @Success 200 {file} binary
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id}/revisions/{revision_id}/download [get]
func (h *Handler) Download(c *gin.Context) {
	h.serveFile(c, true)
}

// View godoc
// @Summary Xem file revision inline
// @Tags documents
// @Security BearerAuth
// @Produce application/octet-stream
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Param revision_id path string true "Revision ID" format(uuid)
// @Success 200 {file} binary
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id}/revisions/{revision_id}/view [get]
func (h *Handler) View(c *gin.Context) {
	h.serveFile(c, false)
}
func (h *Handler) serveFile(c *gin.Context, attachment bool) {
	pid, did, rid, ok := revisionIDs(c)
	if !ok {
		return
	}
	revision, reader, err := h.svc.Download(c.Request.Context(), pid, did, rid)
	if err != nil {
		fail(c, err)
		return
	}
	defer reader.Close()
	disposition := "inline"
	if attachment {
		disposition = "attachment"
	}
	header := map[string]string{
		"Content-Disposition":    mime.FormatMediaType(disposition, map[string]string{"filename": revision.FileName}),
		"X-Content-Type-Options": "nosniff",
	}
	c.DataFromReader(http.StatusOK, revision.SizeBytes, revision.MediaType, reader, header)
}

// Delete godoc
// @Summary Xóa mềm tài liệu
// @Tags documents
// @Security BearerAuth
// @Param id path string true "Project ID" format(uuid)
// @Param document_id path string true "Document ID" format(uuid)
// @Success 204
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/documents/{document_id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	pid, did, ok := documentIDs(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), pid, did); err != nil {
		fail(c, err)
		return
	}
	response.NoContent(c)
}

func documentIDs(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	pid, ok := pathID(c, "id")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	did, ok := pathID(c, "document_id")
	return pid, did, ok
}
func revisionIDs(c *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	pid, did, ok := documentIDs(c)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	rid, ok := pathID(c, "revision_id")
	return pid, did, rid, ok
}
func pathID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		fail(c, apperr.BadRequest("ID không hợp lệ"))
		return uuid.Nil, false
	}
	return id, true
}
func optionalID(s string) (uuid.UUID, bool) {
	if s == "" {
		return uuid.Nil, true
	}
	id, err := uuid.Parse(s)
	return id, err == nil
}
func optionalIDPtr(s string) (*uuid.UUID, bool) {
	if s == "" {
		return nil, true
	}
	id, ok := optionalID(s)
	if !ok {
		return nil, false
	}
	return &id, true
}
func formScope(c *gin.Context) (domain.Scope, bool) {
	return scopeIDs(c, c.PostForm("project_version_id"), c.PostForm("change_request_id"))
}
func scopeIDs(c *gin.Context, v, cr string) (domain.Scope, bool) {
	vid, vok := optionalIDPtr(v)
	cid, cok := optionalIDPtr(cr)
	if !vok || !cok {
		fail(c, apperr.BadRequest("Scope ID không hợp lệ"))
		return domain.Scope{}, false
	}
	return domain.Scope{VersionID: vid, ChangeRequestID: cid}, true
}
func fail(c *gin.Context, err error) { _ = c.Error(err) }
