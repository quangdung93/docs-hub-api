package bootstrap

import (
	"github.com/gin-gonic/gin"

	"github.com/quangdung93/docs-hub-api/internal/config"
	"github.com/quangdung93/docs-hub-api/internal/module/auth"
	"github.com/quangdung93/docs-hub-api/internal/module/chat"
	"github.com/quangdung93/docs-hub-api/internal/module/document"
	"github.com/quangdung93/docs-hub-api/internal/module/project"
	"github.com/quangdung93/docs-hub-api/internal/module/retrieval"
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
		auth.New(auth.Deps{
			DB:         infra.DB,
			JWTManager: infra.JWT,
			AccessTTL:  cfg.JWT.AccessTTL,
			RefreshTTL: cfg.JWT.RefreshTTL,
			// Cookie Secure chỉ bật khi KHÔNG phải local — local chạy HTTP nên
			// bật Secure sẽ khiến trình duyệt bỏ qua cookie.
			SecureCookie: !cfg.App.IsLocal(),
			Cache:        infra.Cache,
		}),
		project.New(project.Deps{
			DB:                 infra.DB,
			Tx:                 infra.Tx,
			Clock:              infra.clock(),
			RAG:                infra.RAG,
			DatasetPrefix:      cfg.RAGFlow.DatasetPrefix,
			ObjectStore:        infra.ObjectStore,
			AvatarMaxBytes:     cfg.Project.AvatarMaxBytes,
			AvatarPresignedTTL: cfg.Project.AvatarPresignedTTL,
		}),
		// file.New(...), notification.New(...), tenant.New(...) — tương lai.
	}
	if infra.ObjectStore != nil {
		modules = append(modules, document.New(document.Deps{
			DB: infra.DB, Tx: infra.Tx, Store: infra.ObjectStore, Clock: infra.clock(),
			BypassProjectACL: cfg.App.IsLocal(),
		}))
	}
	if infra.RAG != nil {
		retrievalModule := retrieval.New(retrieval.Deps{
			DB: infra.DB, RAG: infra.RAG, BypassACL: cfg.App.IsLocal(),
		})
		modules = append(modules, retrievalModule, chat.New(chat.Deps{
			DB: infra.DB, RAG: infra.RAG, Clock: infra.clock(),
		}))
	}
	return modules
}
