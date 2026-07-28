// Package bootstrap là composition root: nạp cấu hình, dựng logger/telemetry/hạ
// tầng/module/server, chạy và graceful shutdown theo thứ tự ngược.
//
// Đây là nơi DUY NHẤT ráp mọi thứ lại (Dependency Injection bằng constructor).
// Đọc file này từ trên xuống là hiểu toàn bộ vòng đời ứng dụng.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/quangdung393/docs-hub-api/internal/config"
	"github.com/quangdung393/docs-hub-api/internal/infrastructure/httpserver"
	"github.com/quangdung393/docs-hub-api/internal/infrastructure/telemetry"
	"github.com/quangdung393/docs-hub-api/internal/middleware"
	"github.com/quangdung393/docs-hub-api/internal/module/health"
)

// Run là điểm vào của ứng dụng: chạy tới khi ctx bị hủy (SIGINT/SIGTERM) rồi
// shutdown gọn gàng. Trả lỗi nếu khởi động hoặc shutdown thất bại.
func Run(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	// 1) Telemetry (tracer + metrics).
	tracer, err := telemetry.NewTracerProvider(ctx, telemetry.TracerConfig{
		Enabled:      cfg.Telemetry.TracingEnabled,
		ServiceName:  cfg.App.Name,
		Environment:  string(cfg.App.Env),
		OTLPEndpoint: cfg.Telemetry.OTLPEndpoint,
		SampleRatio:  cfg.Telemetry.SampleRatio,
	})
	if err != nil {
		return fmt.Errorf("khởi tạo tracer: %w", err)
	}
	metrics := telemetry.NewMetrics(metricsNamespace(cfg.App.Name))

	// 2) Hạ tầng (DB, Redis, MQ, MinIO, JWT, Hasher).
	infra, err := NewInfra(ctx, cfg, log, metrics, tracer)
	if err != nil {
		return err
	}

	// 3) Module nghiệp vụ.
	modules := buildModules(infra)

	// 4) Engine + route.
	extra := buildGlobalInfraMiddleware(cfg, infra)
	engine := httpserver.NewAPIEngine(httpserver.EngineDeps{
		Config: cfg, Logger: log, Metrics: metrics, Extra: extra,
	})
	registerRoutes(engine, cfg, infra, modules)
	registerSwagger(engine, cfg)

	// 5) Hai server: API và Admin.
	checker := health.New(infra.Checkers...)
	apiSrv := httpserver.NewAPIServer(fmt.Sprintf(":%d", cfg.HTTP.APIPort), engine, httpserver.TimeoutSet{
		ReadHeader: cfg.Timeout.ReadHeader,
		Read:       cfg.Timeout.Read,
		Write:      cfg.Timeout.Write,
		Idle:       cfg.Timeout.Idle,
	})
	adminSrv := httpserver.NewAdminServer(httpserver.AdminOptions{
		Addr:        fmt.Sprintf(":%d", cfg.HTTP.AdminPort),
		Registry:    metrics.Registry,
		Liveness:    checker.LivenessHandler(),
		Readiness:   checker.ReadinessHandler(),
		EnablePprof: cfg.HTTP.EnablePprof,
	})

	// 6) Chạy + chờ tín hiệu + shutdown.
	return runServers(ctx, cfg, log, infra, tracer, apiSrv, adminSrv)
}

// runServers chạy 2 server, chờ ctx hủy, rồi shutdown theo thứ tự ngược.
func runServers(
	ctx context.Context, cfg *config.Config, log *zap.Logger,
	infra *Infra, tracer *telemetry.TracerProvider,
	apiSrv, adminSrv *httpserver.Server,
) error {
	serverErr := make(chan error, 2)
	go func() { serverErr <- apiSrv.Start() }()
	go func() { serverErr <- adminSrv.Start() }()

	log.Info("service đã khởi động",
		zap.Int("api_port", cfg.HTTP.APIPort),
		zap.Int("admin_port", cfg.HTTP.AdminPort),
		zap.String("env", string(cfg.App.Env)),
	)

	select {
	case <-ctx.Done():
		log.Info("nhận tín hiệu shutdown, bắt đầu dừng gọn gàng")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("server lỗi: %w", err)
		}
	}

	return shutdown(cfg, log, infra, tracer, apiSrv, adminSrv)
}

// shutdown dừng theo thứ tự ngược: server -> hạ tầng (MQ/Redis/DB) -> tracer.
func shutdown(
	cfg *config.Config, log *zap.Logger,
	infra *Infra, tracer *telemetry.TracerProvider,
	apiSrv, adminSrv *httpserver.Server,
) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()

	// Đóng HTTP server trước để ngừng nhận request mới, chờ request đang bay xong.
	for _, srv := range []*httpserver.Server{apiSrv, adminSrv} {
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("đóng server lỗi", zap.String("server", srv.Name()), zap.Error(err))
		} else {
			log.Info("đã đóng server", zap.String("server", srv.Name()))
		}
	}

	// Đóng hạ tầng (MQ -> Redis -> DB).
	infra.Close(shutdownCtx)
	log.Info("đã đóng hạ tầng")

	// Đóng tracer sau cùng (flush span còn lại).
	if err := tracer.Shutdown(shutdownCtx); err != nil {
		log.Error("đóng tracer lỗi", zap.Error(err))
	}

	log.Info("đã dừng hoàn toàn")
	return nil
}

// buildGlobalInfraMiddleware dựng các middleware toàn cục cần hạ tầng (rate limit).
func buildGlobalInfraMiddleware(cfg *config.Config, infra *Infra) []gin.HandlerFunc {
	if !cfg.RateLimit.Enabled {
		return nil
	}
	return []gin.HandlerFunc{
		middleware.RateLimit(middleware.RateLimiterDeps{
			Cache:             infra.Cache,
			RequestsPerWindow: cfg.RateLimit.RequestsPerWindow,
			Window:            cfg.RateLimit.Window,
			OnFallback:        infra.Metrics.IncRateLimitFallback,
		}),
	}
}

// metricsNamespace chuẩn hóa tên service thành namespace Prometheus hợp lệ.
func metricsNamespace(appName string) string {
	ns := make([]rune, 0, len(appName))
	for _, r := range appName {
		if r == '-' || r == '.' {
			ns = append(ns, '_')
			continue
		}
		ns = append(ns, r)
	}
	return string(ns)
}
