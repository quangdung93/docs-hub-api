// Package document ráp vertical slice upload và quản lý tài liệu.
package document

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	dochttp "github.com/quangdung93/docs-hub-api/internal/module/document/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/document/repository"
	"github.com/quangdung93/docs-hub-api/internal/module/document/usecase"
)

type Deps struct {
	DB               *gorm.DB
	Tx               port.TxManager
	Store            port.ObjectStore
	Clock            port.Clock
	BypassProjectACL bool
}
type Module struct {
	handler *dochttp.Handler
	service *usecase.Service
}

func New(d Deps) *Module {
	repo := repository.New(d.DB)
	service := usecase.New(repo, d.Tx, d.Store, d.Clock, usecase.WithProjectACLBypass(d.BypassProjectACL))
	return &Module{handler: dochttp.New(service), service: service}
}

// Service trả application usecase để MCP dùng lại ACL tài liệu.
func (m *Module) Service() *usecase.Service                   { return m.service }
func (*Module) Name() string                                  { return "document" }
func (m *Module) RegisterRoutes(internal, _ *gin.RouterGroup) { dochttp.Register(internal, m.handler) }
