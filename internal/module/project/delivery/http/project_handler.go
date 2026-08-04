// Package http là tầng delivery của module project: bind/validate request,
// gọi service, trả envelope. Handler phải MỎNG — không chứa business logic.
// RBAC theo vai trò trong dự án được gắn ở route.go qua middleware.RequireProjectRole
// (không ở trong handler).
package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/common/validatorx"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/project/usecase"
)

// Register gắn các route của module project vào router group nội bộ.
// memberRepo dùng để middleware.RequireProjectRole tự tra cứu vai trò của actor
// trong dự án (path param "id") — xem internal/middleware/project_auth.go.
//
// Quy ước phân quyền:
//   - GET/POST /projects              : chỉ cần đã xác thực (không gắn với 1 dự án cụ thể).
//   - PATCH    /projects/:id          : owner, editor.
//   - DELETE   /projects/:id          : chỉ owner.
//   - GET      /projects/:id/members  : owner, editor, viewer (mọi thành viên active).
//   - POST|PATCH|DELETE .../members.. : chỉ owner (quản lý thành viên).
//   - POST .../members/me/accept      : chỉ cần đã xác thực (tự accept lời mời của mình).
func Register(rg *gin.RouterGroup, h *Handler, memberRepo domain.ProjectMemberRepository) {
	onlyOwner := middleware.RequireProjectRole(memberRepo, domain.RoleOwner)
	ownerOrEditor := middleware.RequireProjectRole(memberRepo, domain.RoleOwner, domain.RoleEditor)
	anyActiveMember := middleware.RequireProjectRole(memberRepo, domain.RoleOwner, domain.RoleEditor, domain.RoleViewer)

	projects := rg.Group("/projects")
	{
		projects.GET("", h.List)
		projects.POST("", h.Create)
		projects.PATCH("/:id", ownerOrEditor, h.Update)
		projects.DELETE("/:id", onlyOwner, h.Delete)

		projects.GET("/:id/members", anyActiveMember, h.ListMembers)
		projects.POST("/:id/members", onlyOwner, h.InviteMember)
		projects.POST("/:id/members/me/accept", h.AcceptInvite)
		projects.PATCH("/:id/members/:userId", onlyOwner, h.ChangeMemberRole)
		projects.DELETE("/:id/members/:userId", onlyOwner, h.RemoveMember)
	}
}

// Handler xử lý HTTP cho module project. Chỉ giữ tham chiếu service — không state khác.
type Handler struct {
	svc usecase.Service
}

// NewHandler tạo Handler.
func NewHandler(svc usecase.Service) *Handler {
	return &Handler{svc: svc}
}

// --- DTO request ---

// ProjectSettingsRequest là body cấu hình RAG của dự án.
type ProjectSettingsRequest struct {
	Model          string   `json:"model"           binding:"omitempty"`
	TopK           int      `json:"top_k"           binding:"omitempty,min=1"`
	ChunkSize      int      `json:"chunk_size"      binding:"omitempty,min=1"`
	AllowedFormats []string `json:"allowed_formats" binding:"omitempty,dive,required"`
}

func (r ProjectSettingsRequest) toDomain() domain.ProjectSettings {
	return domain.ProjectSettings{
		Model:          r.Model,
		TopK:           r.TopK,
		ChunkSize:      r.ChunkSize,
		AllowedFormats: r.AllowedFormats,
	}
}

// CreateProjectRequest là body tạo dự án.
type CreateProjectRequest struct {
	Name        string                  `json:"name"        binding:"required,min=1,max=255"`
	Description string                  `json:"description" binding:"omitempty,max=2000"`
	Settings    *ProjectSettingsRequest `json:"settings"    binding:"omitempty"`
}

// UpdateProjectRequest là body cập nhật dự án. Field nil = giữ nguyên (PATCH).
type UpdateProjectRequest struct {
	Name        *string                 `json:"name"        binding:"omitempty,min=1,max=255"`
	Description *string                 `json:"description" binding:"omitempty,max=2000"`
	Settings    *ProjectSettingsRequest `json:"settings"    binding:"omitempty"`
}

// ListProjectQuery là query string liệt kê dự án (nhúng pagination.Query).
type ListProjectQuery struct {
	pagination.Query
}

// InviteMemberRequest là body mời thành viên mới.
type InviteMemberRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
	Role   string `json:"role"    binding:"required,oneof=editor viewer"`
}

// ChangeMemberRoleRequest là body đổi vai trò thành viên.
type ChangeMemberRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=editor viewer"`
}

// --- DTO response ---

// ProjectSettingsResponse là cấu hình RAG trả về client.
type ProjectSettingsResponse struct {
	Model          string   `json:"model"`
	TopK           int      `json:"top_k"`
	ChunkSize      int      `json:"chunk_size"`
	AllowedFormats []string `json:"allowed_formats"`
}

// ProjectResponse là biểu diễn dự án trả ra client.
type ProjectResponse struct {
	ID          string                  `json:"id"`
	OwnerID     string                  `json:"owner_id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Status      string                  `json:"status"`
	Settings    ProjectSettingsResponse `json:"settings"`
	CreatedAt   time.Time               `json:"created_at"`
}

func toProjectResponse(p *domain.Project) ProjectResponse {
	return ProjectResponse{
		ID:          p.ID.String(),
		OwnerID:     p.OwnerID.String(),
		Name:        p.Name,
		Description: p.Description,
		Status:      p.Status,
		Settings: ProjectSettingsResponse{
			Model:          p.Settings.Model,
			TopK:           p.Settings.TopK,
			ChunkSize:      p.Settings.ChunkSize,
			AllowedFormats: p.Settings.AllowedFormats,
		},
		CreatedAt: p.CreatedAt,
	}
}

func toProjectResponseList(projects []domain.Project) []ProjectResponse {
	out := make([]ProjectResponse, 0, len(projects))
	for i := range projects {
		out = append(out, toProjectResponse(&projects[i]))
	}
	return out
}

// ProjectMemberResponse là biểu diễn thành viên dự án trả ra client.
type ProjectMemberResponse struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	UserID    string     `json:"user_id"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	InvitedAt time.Time  `json:"invited_at"`
	JoinedAt  *time.Time `json:"joined_at,omitempty"`
}

func toMemberResponse(m *domain.ProjectMember) ProjectMemberResponse {
	return ProjectMemberResponse{
		ID:        m.ID.String(),
		ProjectID: m.ProjectID.String(),
		UserID:    m.UserID.String(),
		Role:      string(m.Role),
		Status:    string(m.Status),
		InvitedAt: m.InvitedAt,
		JoinedAt:  m.JoinedAt,
	}
}

func toMemberResponseList(members []domain.ProjectMember) []ProjectMemberResponse {
	out := make([]ProjectMemberResponse, 0, len(members))
	for i := range members {
		out = append(out, toMemberResponse(&members[i]))
	}
	return out
}

// --- Helpers ---

// bindBody bind JSON body; lỗi -> REQ_400 kèm details. Trả false nếu lỗi.
func bindBody(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		_ = c.Error(apperr.BadRequest("Dữ liệu không hợp lệ").WithDetails(validatorx.ToDetails(err)))
		return false
	}
	return true
}

// parseUUIDParam đọc và parse UUID từ path param. Lỗi -> REQ_400.
func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		_ = c.Error(apperr.BadRequest("ID không hợp lệ").
			WithDetails(map[string]string{name: "phải là UUID hợp lệ"}))
		return uuid.Nil, false
	}
	return id, true
}

// currentUserID lấy user_id của actor đã xác thực (đặt bởi middleware.Auth).
func currentUserID(c *gin.Context) (uuid.UUID, bool) {
	actor, ok := contextx.ActorFrom(c.Request.Context())
	if !ok {
		_ = c.Error(apperr.Unauthorized("Chưa xác thực"))
		return uuid.Nil, false
	}
	id, err := uuid.Parse(actor.UserID)
	if err != nil {
		_ = c.Error(apperr.Unauthorized("Định danh người dùng không hợp lệ"))
		return uuid.Nil, false
	}
	return id, true
}

// --- Handlers: Project ---

// Create godoc
// @Summary  Tạo dự án mới
// @Tags     projects
// @Accept   json
// @Produce  json
// @Param    body  body      CreateProjectRequest  true  "Thông tin dự án"
// @Success  201   {object}  response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/projects [post]
func (h *Handler) Create(c *gin.Context) {
	ownerID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req CreateProjectRequest
	if !bindBody(c, &req) {
		return
	}
	settings := domain.ProjectSettings{}
	if req.Settings != nil {
		settings = req.Settings.toDomain()
	}
	project, err := h.svc.Create(c.Request.Context(), usecase.CreateProjectInput{
		OwnerID:     ownerID,
		Name:        req.Name,
		Description: req.Description,
		Settings:    settings,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.Created(c, toProjectResponse(project))
}

// List godoc
// @Summary  Danh sách dự án của người dùng hiện tại
// @Tags     projects
// @Produce  json
// @Param    page     query     int     false  "Trang"
// @Param    limit    query     int     false  "Số bản ghi/trang"
// @Param    sort_by  query     string  false  "Cột sắp xếp"
// @Param    order    query     string  false  "asc|desc"
// @Success  200      {object}  response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/projects [get]
func (h *Handler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var q ListProjectQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		_ = c.Error(apperr.BadRequest("Tham số truy vấn không hợp lệ").WithDetails(validatorx.ToDetails(err)))
		return
	}
	projects, meta, err := h.svc.List(c.Request.Context(), userID, q.Query)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OKPaged(c, toProjectResponseList(projects), meta)
}

// Update godoc
// @Summary  Cập nhật thông tin/cấu hình dự án
// @Tags     projects
// @Accept   json
// @Produce  json
// @Param    id    path      string                 true  "Project ID"
// @Param    body  body      UpdateProjectRequest   true  "Thông tin cập nhật"
// @Success  200   {object}  response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/projects/{id} [patch]
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req UpdateProjectRequest
	if !bindBody(c, &req) {
		return
	}
	var settings *domain.ProjectSettings
	if req.Settings != nil {
		s := req.Settings.toDomain()
		settings = &s
	}
	project, err := h.svc.Update(c.Request.Context(), usecase.UpdateProjectInput{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Settings:    settings,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, toProjectResponse(project))
}

// Delete godoc
// @Summary  Xóa dự án (chỉ Owner)
// @Tags     projects
// @Produce  json
// @Param    id  path  string  true  "Project ID"
// @Success  204
// @Security BearerAuth
// @Router   /internal/api/v1/projects/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		_ = c.Error(err)
		return
	}
	response.NoContent(c)
}

// --- Handlers: Project members ---

// ListMembers godoc
// @Summary  Danh sách thành viên dự án
// @Tags     projects
// @Produce  json
// @Param    id  path      string  true  "Project ID"
// @Success  200 {object}  response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/projects/{id}/members [get]
func (h *Handler) ListMembers(c *gin.Context) {
	projectID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	members, err := h.svc.ListMembers(c.Request.Context(), projectID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, toMemberResponseList(members))
}

// InviteMember godoc
// @Summary  Mời thành viên mới vào dự án
// @Tags     projects
// @Accept   json
// @Produce  json
// @Param    id    path      string                true  "Project ID"
// @Param    body  body      InviteMemberRequest   true  "Thông tin mời"
// @Success  201   {object}  response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/projects/{id}/members [post]
func (h *Handler) InviteMember(c *gin.Context) {
	projectID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req InviteMemberRequest
	if !bindBody(c, &req) {
		return
	}
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		_ = c.Error(apperr.BadRequest("user_id không hợp lệ"))
		return
	}
	member, err := h.svc.InviteMember(c.Request.Context(), usecase.InviteMemberInput{
		ProjectID: projectID,
		UserID:    userID,
		Role:      domain.Role(req.Role),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.Created(c, toMemberResponse(member))
}

// ChangeMemberRole godoc
// @Summary  Đổi vai trò của thành viên
// @Tags     projects
// @Accept   json
// @Produce  json
// @Param    id      path      string                    true  "Project ID"
// @Param    userId  path      string                    true  "User ID"
// @Param    body    body      ChangeMemberRoleRequest  true  "Vai trò mới"
// @Success  200     {object}  response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/projects/{id}/members/{userId} [patch]
func (h *Handler) ChangeMemberRole(c *gin.Context) {
	projectID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	userID, ok := parseUUIDParam(c, "userId")
	if !ok {
		return
	}
	var req ChangeMemberRoleRequest
	if !bindBody(c, &req) {
		return
	}
	member, err := h.svc.ChangeMemberRole(c.Request.Context(), usecase.ChangeMemberRoleInput{
		ProjectID: projectID,
		UserID:    userID,
		Role:      domain.Role(req.Role),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, toMemberResponse(member))
}

// RemoveMember godoc
// @Summary  Gỡ thành viên khỏi dự án
// @Tags     projects
// @Produce  json
// @Param    id      path  string  true  "Project ID"
// @Param    userId  path  string  true  "User ID"
// @Success  204
// @Security BearerAuth
// @Router   /internal/api/v1/projects/{id}/members/{userId} [delete]
func (h *Handler) RemoveMember(c *gin.Context) {
	projectID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	userID, ok := parseUUIDParam(c, "userId")
	if !ok {
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), projectID, userID); err != nil {
		_ = c.Error(err)
		return
	}
	response.NoContent(c)
}

// AcceptInvite godoc
// @Summary  Xác nhận lời mời tham gia dự án
// @Tags     projects
// @Produce  json
// @Param    id  path      string  true  "Project ID"
// @Success  200 {object}  response.Envelope
// @Security BearerAuth
// @Router   /internal/api/v1/projects/{id}/members/me/accept [post]
func (h *Handler) AcceptInvite(c *gin.Context) {
	projectID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	member, err := h.svc.AcceptInvite(c.Request.Context(), projectID, userID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, toMemberResponse(member))
}
