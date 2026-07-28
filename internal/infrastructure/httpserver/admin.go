package httpserver

import (
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AdminOptions cấu hình admin server (:9090) — chỉ dùng nội bộ, không expose ra ngoài.
type AdminOptions struct {
	Addr        string
	Registry    *prometheus.Registry
	Liveness    http.HandlerFunc
	Readiness   http.HandlerFunc
	EnablePprof bool
}

// NewAdminServer dựng server nội bộ phục vụ metrics, health check và (tùy chọn) pprof.
// Tách khỏi API server để KHÔNG expose metrics/pprof ra internet.
func NewAdminServer(opts AdminOptions) *Server {
	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.HandlerFor(opts.Registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", opts.Liveness) // liveness: tiến trình còn sống?
	mux.HandleFunc("/readyz", opts.Readiness) // readiness: dependency đã sẵn sàng?

	if opts.EnablePprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	return New(Options{
		Name:              "admin",
		Addr:              opts.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second, // profile có thể lâu
		IdleTimeout:       60 * time.Second,
	})
}
