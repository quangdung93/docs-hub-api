// Package chat ráp vertical slice hội thoại tra cứu có citation.
package chat

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	chathttp "github.com/quangdung93/docs-hub-api/internal/module/chat/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/chat/repository"
	chatuc "github.com/quangdung93/docs-hub-api/internal/module/chat/usecase"
	retrievalrepo "github.com/quangdung93/docs-hub-api/internal/module/retrieval/repository"
)

type Deps struct {
	DB    *gorm.DB
	RAG   port.RAGClient
	Clock port.Clock
}

type Module struct {
	handler *chathttp.Handler
	service *chatuc.Service
}

func New(deps Deps) *Module {
	repo := repository.New(deps.DB)
	scopeRepo := retrievalrepo.New(deps.DB)
	service := chatuc.New(repo, scopeRepo, deps.RAG, deps.Clock)
	return &Module{handler: chathttp.New(service), service: service}
}

// Service trả application usecase để MCP dùng lại ACL và citation.
func (m *Module) Service() *chatuc.Service { return m.service }

func (*Module) Name() string { return "chat" }

func (m *Module) RegisterRoutes(internal, _ *gin.RouterGroup) {
	chathttp.Register(internal, m.handler)
}
