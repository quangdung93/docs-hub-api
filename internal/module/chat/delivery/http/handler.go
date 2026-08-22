// Package http cung cấp REST delivery cho conversation và message.
package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/internal/module/chat/usecase"
	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

type Handler struct{ service *usecase.Service }

func New(service *usecase.Service) *Handler { return &Handler{service: service} }

type ScopeRequest struct {
	Mode             string   `json:"mode" binding:"required"`
	VersionIDs       []string `json:"version_ids"`
	ChangeRequestIDs []string `json:"change_request_ids"`
}

type CreateRequest struct {
	Title string        `json:"title"`
	Scope *ScopeRequest `json:"scope"`
}

// Create godoc
// @Summary Tạo conversation trong project
// @Tags conversations
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param body body CreateRequest true "Tiêu đề và scope tùy chọn"
// @Success 201 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/conversations [post]
func (h *Handler) Create(c *gin.Context) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request CreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(apperr.BadRequest("Dữ liệu conversation không hợp lệ"))
		return
	}
	scope, err := optionalScope(request.Scope)
	if err != nil {
		_ = c.Error(err)
		return
	}
	conversation, err := h.service.Create(c.Request.Context(), usecase.CreateInput{
		ProjectID: projectID, Title: request.Title, Scope: scope,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.Created(c, conversation)
}

// List godoc
// @Summary Liệt kê conversation của actor
// @Tags conversations
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param page query int false "Trang"
// @Param limit query int false "Số bản ghi/trang"
// @Success 200 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/conversations [get]
func (h *Handler) List(c *gin.Context) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var page pagination.Query
	if err := c.ShouldBindQuery(&page); err != nil {
		_ = c.Error(apperr.BadRequest("Phân trang không hợp lệ"))
		return
	}
	items, meta, err := h.service.List(c.Request.Context(), projectID, page)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OKPaged(c, items, meta)
}

// Get godoc
// @Summary Xem conversation, messages và citations
// @Tags conversations
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param conversation_id path string true "Conversation ID" format(uuid)
// @Success 200 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/conversations/{conversation_id} [get]
func (h *Handler) Get(c *gin.Context) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return
	}
	conversationID, ok := parseID(c, "conversation_id")
	if !ok {
		return
	}
	conversation, err := h.service.Get(c.Request.Context(), projectID, conversationID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, conversation)
}

type AskRequest struct {
	Question string        `json:"question" binding:"required"`
	Scope    *ScopeRequest `json:"scope"`
}

// Ask godoc
// @Summary Hỏi đáp trên conversation với citation
// @Description RAGFlow Chat sinh answer và citations; nếu không truyền scope, truy vấn tất cả version và change request của project.
// @Tags conversations
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID" format(uuid)
// @Param conversation_id path string true "Conversation ID" format(uuid)
// @Param body body AskRequest true "Câu hỏi và scope tùy chọn"
// @Success 200 {object} response.Envelope
// @Router /internal/api/v1/projects/{id}/conversations/{conversation_id}/messages [post]
func (h *Handler) Ask(c *gin.Context) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return
	}
	conversationID, ok := parseID(c, "conversation_id")
	if !ok {
		return
	}
	var request AskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(apperr.BadRequest("Dữ liệu message không hợp lệ"))
		return
	}
	scope, err := optionalScope(request.Scope)
	if err != nil {
		_ = c.Error(err)
		return
	}
	answer, err := h.service.Ask(c.Request.Context(), usecase.AskInput{
		ProjectID: projectID, ConversationID: conversationID,
		Question: request.Question, Scope: scope,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	response.OK(c, answer)
}

func parseID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		_ = c.Error(apperr.BadRequest("ID không hợp lệ"))
		return uuid.Nil, false
	}
	return id, true
}

func optionalScope(input *ScopeRequest) (*retrievaldomain.Scope, error) {
	if input == nil {
		return nil, nil
	}
	scope := &retrievaldomain.Scope{Mode: input.Mode}
	for _, raw := range input.VersionIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, apperr.BadRequest("Project version ID không hợp lệ")
		}
		scope.VersionIDs = append(scope.VersionIDs, id)
	}
	for _, raw := range input.ChangeRequestIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, apperr.BadRequest("Change request ID không hợp lệ")
		}
		scope.ChangeRequestIDs = append(scope.ChangeRequestIDs, id)
	}
	if !scope.Valid() {
		return nil, apperr.BadRequest("Scope không hợp lệ")
	}
	return scope, nil
}
