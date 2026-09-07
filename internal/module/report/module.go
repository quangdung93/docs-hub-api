// Package report ráp vertical slice xuất báo cáo dự án (UAT Report / Project
// Planning / Testcase — SRS v1.1 mục IX/X).
package report

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	reporthttp "github.com/quangdung93/docs-hub-api/internal/module/report/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/report/repository"
	"github.com/quangdung93/docs-hub-api/internal/module/report/usecase"
	retrievalrepo "github.com/quangdung93/docs-hub-api/internal/module/retrieval/repository"
)

type Deps struct {
	DB               *gorm.DB
	Tx               port.TxManager
	RAG              port.RAGClient
	Store            port.ObjectStore
	Clock            port.Clock
	BypassProjectACL bool
}

type Module struct {
	handler *reporthttp.Handler
	service *usecase.Service
}

func New(d Deps) *Module {
	repo := repository.New(d.DB)
	scopeRepo := retrievalrepo.New(d.DB)
	service := usecase.New(
		repo, scopeRepo, d.Tx, d.RAG, d.Store, d.Clock, usecase.WithProjectACLBypass(d.BypassProjectACL))
	return &Module{handler: reporthttp.New(service), service: service}
}

func (m *Module) Service() *usecase.Service { return m.service }

func (*Module) Name() string { return "report" }

func (m *Module) RegisterRoutes(internal, _ *gin.RouterGroup) {
	reporthttp.Register(internal, m.handler)
}
