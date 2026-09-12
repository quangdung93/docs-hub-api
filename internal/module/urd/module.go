// Package urd ráp vertical slice "Nhận diện & Phân tích Edge Case cho URD
// (AI)" (URD v1.2 mục XI).
package urd

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	documentusecase "github.com/quangdung93/docs-hub-api/internal/module/document/usecase"
	urdhttp "github.com/quangdung93/docs-hub-api/internal/module/urd/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/urd/repository"
	"github.com/quangdung93/docs-hub-api/internal/module/urd/usecase"
)

type Deps struct {
	DB               *gorm.DB
	Tx               port.TxManager
	RAG              port.RAGClient
	Store            port.ObjectStore
	Clock            port.Clock
	DocumentService  *documentusecase.Service
	BypassProjectACL bool
}

type Module struct {
	handler *urdhttp.Handler
	service *usecase.Service
}

func New(d Deps) *Module {
	repo := repository.New(d.DB)
	service := usecase.New(
		repo, d.Tx, d.RAG, d.Store, d.Clock, d.DocumentService, usecase.WithProjectACLBypass(d.BypassProjectACL))
	return &Module{handler: urdhttp.New(service), service: service}
}

func (m *Module) Service() *usecase.Service { return m.service }

func (*Module) Name() string { return "urd" }

func (m *Module) RegisterRoutes(internal, _ *gin.RouterGroup) {
	urdhttp.Register(internal, m.handler)
}
