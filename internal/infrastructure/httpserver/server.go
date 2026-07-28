package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Server bọc http.Server với timeout cấu hình sẵn và graceful shutdown.
type Server struct {
	httpServer *http.Server
	name       string
}

// Options cấu hình một Server.
type Options struct {
	Name              string // "api" | "admin" — chỉ để log
	Addr              string // ":8080"
	Handler           http.Handler
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// New tạo Server từ Options.
func New(opts Options) *Server {
	return &Server{
		name: opts.Name,
		httpServer: &http.Server{
			Addr:              opts.Addr,
			Handler:           opts.Handler,
			ReadHeaderTimeout: opts.ReadHeaderTimeout,
			ReadTimeout:       opts.ReadTimeout,
			WriteTimeout:      opts.WriteTimeout,
			IdleTimeout:       opts.IdleTimeout,
		},
	}
}

// Start chạy server (blocking). Trả nil khi được shutdown chủ động.
func (s *Server) Start() error {
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server %s dừng bất thường: %w", s.name, err)
	}
	return nil
}

// Shutdown dừng server có thời hạn, chờ request đang xử lý hoàn tất.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server %s: %w", s.name, err)
	}
	return nil
}

// Name trả về tên server (dùng khi log).
func (s *Server) Name() string { return s.name }

// NewAPIServer là tiện ích tạo API server từ gin engine + config timeout.
func NewAPIServer(addr string, engine *gin.Engine, t TimeoutSet) *Server {
	return New(Options{
		Name:              "api",
		Addr:              addr,
		Handler:           engine,
		ReadHeaderTimeout: t.ReadHeader,
		ReadTimeout:       t.Read,
		WriteTimeout:      t.Write,
		IdleTimeout:       t.Idle,
	})
}

// TimeoutSet gom các timeout của HTTP server (tách khỏi config để httpserver
// không phụ thuộc ngược lên package config).
type TimeoutSet struct {
	ReadHeader time.Duration
	Read       time.Duration
	Write      time.Duration
	Idle       time.Duration
}
