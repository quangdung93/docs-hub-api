// Package project ráp các tầng của module project thành một đơn vị đăng ký route.
//
// Đây là nơi DUY NHẤT nối repository -> usecase -> handler của feature. Bootstrap
// chỉ cần gọi New(deps) và RegisterRoutes — không cần biết chi tiết bên trong.
package project

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	projecthttp "github.com/quangdung93/docs-hub-api/internal/module/project/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/project/repository"
	"github.com/quangdung93/docs-hub-api/internal/module/project/usecase"
)

// Deps là phụ thuộc hạ tầng mà module project cần (bootstrap cung cấp).
type Deps struct {
	DB    *gorm.DB
	Tx    port.TxManager
	Clock port.Clock
}

// Module là đơn vị đăng ký route của feature project.
type Module struct {
	handler    *projecthttp.Handler
	memberRepo domain.ProjectMemberRepository
}

// New dựng toàn bộ chuỗi phụ thuộc của module project.
func New(d Deps) *Module {
	projectRepo := repository.NewProjectRepository(d.DB)
	memberRepo := repository.NewProjectMemberRepository(d.DB)
	svc := usecase.NewService(usecase.Deps{
		ProjectRepo: projectRepo,
		MemberRepo:  memberRepo,
		Tx:          d.Tx,
		Clock:       d.Clock,
	})
	return &Module{
		handler:    projecthttp.NewHandler(svc),
		memberRepo: memberRepo,
	}
}

// Name trả về tên module (dùng khi log).
func (m *Module) Name() string { return "project" }

// RegisterRoutes gắn route của module vào các group internal/public.
// Module project chỉ dùng group internal (yêu cầu xác thực).
func (m *Module) RegisterRoutes(internal, _ *gin.RouterGroup) {
	projecthttp.Register(internal, m.handler, m.memberRepo)
}
