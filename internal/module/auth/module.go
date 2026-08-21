package auth

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/pkg/jwt"

	"github.com/quangdung93/docs-hub-api/internal/module/auth/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/repository/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/auth/usecase"
)

type Deps struct {
	DB         *gorm.DB
	JWTManager *jwt.Manager
	// AccessTTL/RefreshTTL lấy từ config để cookie, JWT và session cùng một hạn.
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	// SecureCookie bật cờ Secure của cookie — chỉ true khi chạy sau HTTPS.
	SecureCookie bool
}

type Module struct {
	handler *http.AuthHandler
}

func New(d Deps) *Module {
	userRepo := postgres.NewUserRepository(d.DB)
	sessionRepo := postgres.NewSessionRepository(d.DB)
	svc := usecase.NewAuthUseCase(userRepo, sessionRepo, d.JWTManager, d.AccessTTL, d.RefreshTTL)
	return &Module{handler: http.NewAuthHandler(svc, d.SecureCookie)}
}

func (m *Module) Name() string { return "auth" }

func (m *Module) RegisterRoutes(internal, public *gin.RouterGroup) {
	http.Register(internal, public, m.handler)
}
