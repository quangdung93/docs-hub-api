package bootstrap

import (
	"github.com/gin-gonic/gin"

	"github.com/quangdung93/docs-hub-api/internal/config"
	"github.com/quangdung93/docs-hub-api/internal/module/document"
	"github.com/quangdung93/docs-hub-api/internal/module/user"
)

// Module là hợp đồng mà mọi feature phải thỏa để bootstrap đăng ký route.
// Thêm feature mới = thêm 1 dòng trong buildModules — không đụng phần còn lại.
type Module interface {
	Name() string
	RegisterRoutes(internal, public *gin.RouterGroup)
}

// buildModules dựng danh sách module từ hạ tầng. Đây là nơi DUY NHẤT bootstrap
// biết về từng feature cụ thể.
func buildModules(cfg *config.Config, infra *Infra) []Module {
	modules := []Module{
		user.New(user.Deps{
			DB:        infra.DB,
			Tx:        infra.Tx,
			Cache:     infra.Cache,
			Publisher: infra.Publisher,
			Hasher:    infra.Hasher,
			Clock:     infra.clock(),
		}),
	}
	if infra.ObjectStore != nil {
		modules = append(modules, document.New(document.Deps{
			DB: infra.DB, Tx: infra.Tx, Store: infra.ObjectStore, Clock: infra.clock(),
			BypassProjectACL: cfg.App.IsLocal(),
		}))
	}
	return modules
}
