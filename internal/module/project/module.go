// Package project ráp vertical slice quản lý project và RAGFlow dataset.
package project

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	projecthttp "github.com/quangdung93/docs-hub-api/internal/module/project/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/project/repository"
	"github.com/quangdung93/docs-hub-api/internal/module/project/usecase"
)

type Deps struct {
	DB            *gorm.DB
	Tx            port.TxManager
	RAG           port.RAGClient
	Clock         port.Clock
	DatasetPrefix string
}

type Module struct{ handler *projecthttp.Handler }

func New(deps Deps) *Module {
	service := usecase.New(repository.New(deps.DB), deps.Tx, deps.RAG, deps.Clock, deps.DatasetPrefix)
	return &Module{handler: projecthttp.New(service)}
}

func (*Module) Name() string { return "project" }
func (m *Module) RegisterRoutes(internal, _ *gin.RouterGroup) {
	projecthttp.Register(internal, m.handler)
}
