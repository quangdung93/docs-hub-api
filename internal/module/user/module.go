// Package user ráp các tầng của module user thành một đơn vị đăng ký route.
//
// Đây là nơi DUY NHẤT nối repository -> usecase -> handler của feature. Bootstrap
// chỉ cần gọi New(deps) và RegisterRoutes — không cần biết chi tiết bên trong.
package user

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	userhttp "github.com/quangdung93/docs-hub-api/internal/module/user/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/user/repository"
	"github.com/quangdung93/docs-hub-api/internal/module/user/usecase"
)

// Deps là phụ thuộc hạ tầng mà module user cần (bootstrap cung cấp).
type Deps struct {
	DB        *gorm.DB
	Tx        port.TxManager
	Cache     port.Cache
	Publisher port.Publisher
	Hasher    usecase.PasswordHasher
	Clock     port.Clock
}

// Module là đơn vị đăng ký route của feature user.
type Module struct {
	handler *userhttp.Handler
}

// New dựng toàn bộ chuỗi phụ thuộc của module user.
func New(d Deps) *Module {
	repo := repository.New(d.DB)
	svc := usecase.NewService(usecase.Deps{
		Repo:      repo,
		Tx:        d.Tx,
		Cache:     d.Cache,
		Publisher: d.Publisher,
		Hasher:    d.Hasher,
		Clock:     d.Clock,
	})
	return &Module{handler: userhttp.NewHandler(svc)}
}

// Name trả về tên module (dùng khi log).
func (m *Module) Name() string { return "user" }

// RegisterRoutes gắn route của module vào các group internal/public.
// Module user chỉ dùng group internal (yêu cầu xác thực).
func (m *Module) RegisterRoutes(internal, _ *gin.RouterGroup) {
	userhttp.Register(internal, m.handler)
}
