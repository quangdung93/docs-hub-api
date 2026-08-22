// Package retrieval ráp vertical slice retrieval có project/version ACL.
package retrieval

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	retrievalhttp "github.com/quangdung93/docs-hub-api/internal/module/retrieval/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/retrieval/repository"
	"github.com/quangdung93/docs-hub-api/internal/module/retrieval/usecase"
)

type Deps struct {
	DB        *gorm.DB
	RAG       port.RAGClient
	BypassACL bool
}

type Module struct {
	handler *retrievalhttp.Handler
	service *usecase.Service
}

func New(deps Deps) *Module {
	repo := repository.New(deps.DB)
	service := usecase.New(repo, deps.RAG, deps.BypassACL)
	return &Module{handler: retrievalhttp.New(service), service: service}
}

func (m *Module) Service() *usecase.Service { return m.service }

func (*Module) Name() string { return "retrieval" }
func (m *Module) RegisterRoutes(internal, _ *gin.RouterGroup) {
	retrievalhttp.Register(internal, m.handler)
}
