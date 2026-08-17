package http

import (
	"github.com/gin-gonic/gin"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/common/validatorx"
	"github.com/quangdung93/docs-hub-api/internal/module/user/usecase"
)

// Handler xử lý HTTP cho module user. Chỉ giữ tham chiếu service — không state khác.
type Handler struct {
	svc usecase.Service
}

// NewHandler tạo Handler.
func NewHandler(svc usecase.Service) *Handler {
	return &Handler{svc: svc}
}

// Create godoc
// @Summary  Tạo người dùng mới
// @Tags     users
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body  body      CreateUserRequest  true  "Thông tin người dùng"
// @Success  201   {object}  response.Envelope
// @Router   /internal/api/v1/users [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateUserRequest
	if !bindBody(c, &req) {
		return
	}
	user, err := h.svc.Create(c.Request.Context(), req.toInput())
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.Created(c, toResponse(user))
}

// GetByID godoc
// @Summary  Lấy chi tiết người dùng
// @Tags     users
// @Security BearerAuth
// @Produce  json
// @Param    id   path      string  true  "User ID (UUID)"
// @Success  200  {object}  response.Envelope
// @Router   /internal/api/v1/users/{id} [get]
func (h *Handler) GetByID(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	user, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, toResponse(user))
}

// List godoc
// @Summary  Danh sách người dùng (phân trang)
// @Tags     users
// @Security BearerAuth
// @Produce  json
// @Param    page      query     int     false  "Trang"
// @Param    limit     query     int     false  "Số bản ghi/trang"
// @Param    keyword   query     string  false  "Từ khóa tìm email/tên"
// @Param    status    query     string  false  "Lọc trạng thái"
// @Param    sort_by   query     string  false  "Cột sắp xếp"
// @Param    order     query     string  false  "asc|desc"
// @Success  200       {object}  response.Envelope
// @Router   /internal/api/v1/users [get]
func (h *Handler) List(c *gin.Context) {
	var q ListUserQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		_ = c.Error(apperr.BadRequest("Tham số truy vấn không hợp lệ").WithDetails(validatorx.ToDetails(err)))
		return
	}
	users, meta, err := h.svc.List(c.Request.Context(), q.toInput())
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OKPaged(c, toResponseList(users), meta)
}

// Update godoc
// @Summary  Cập nhật hồ sơ người dùng
// @Tags     users
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id    path      string             true  "User ID"
// @Param    body  body      UpdateUserRequest  true  "Thông tin cập nhật"
// @Success  200   {object}  response.Envelope
// @Router   /internal/api/v1/users/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req UpdateUserRequest
	if !bindBody(c, &req) {
		return
	}
	user, err := h.svc.Update(c.Request.Context(), usecase.UpdateInput{
		ID: id, FullName: req.FullName, Version: req.Version,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, toResponse(user))
}

// UpdateStatus godoc
// @Summary  Đổi trạng thái người dùng
// @Tags     users
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id    path      string               true  "User ID"
// @Param    body  body      UpdateStatusRequest  true  "Trạng thái mới"
// @Success  200   {object}  response.Envelope
// @Router   /internal/api/v1/users/{id}/status [patch]
func (h *Handler) UpdateStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req UpdateStatusRequest
	if !bindBody(c, &req) {
		return
	}
	user, err := h.svc.SetStatus(c.Request.Context(), usecase.SetStatusInput{
		ID: id, Status: statusFromString(req.Status), Version: req.Version,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, toResponse(user))
}

// Delete godoc
// @Summary  Xóa mềm người dùng
// @Tags     users
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id    path      string             true  "User ID"
// @Param    body  body      DeleteUserRequest  true  "Version"
// @Success  204
// @Router   /internal/api/v1/users/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req DeleteUserRequest
	if !bindBody(c, &req) {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id, req.Version); err != nil {
		_ = c.Error(err)
		return
	}
	response.NoContent(c)
}

// CheckEmail godoc
// @Summary  Kiểm tra email còn trống
// @Tags     users
// @Security BearerAuth
// @Produce  json
// @Param    email  query     string  true  "Email cần kiểm tra"
// @Success  200    {object}  response.Envelope
// @Router   /internal/api/v1/users/check-email [get]
func (h *Handler) CheckEmail(c *gin.Context) {
	var q CheckEmailQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		_ = c.Error(apperr.BadRequest("Email không hợp lệ").WithDetails(validatorx.ToDetails(err)))
		return
	}
	available, err := h.svc.EmailAvailable(c.Request.Context(), q.Email)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, gin.H{"email": q.Email, "available": available})
}
