// Package telemetry cấu hình OpenTelemetry tracing và Prometheus metrics.
//
// Dùng registry Prometheus RIÊNG (không phải DefaultRegisterer toàn cục) để
// tránh biến global và cho phép test độc lập.
package telemetry

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics gom registry và các collector HTTP/DB.
type Metrics struct {
	Registry *prometheus.Registry

	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
	httpInFlight        prometheus.Gauge
	rateLimitFallback   prometheus.Counter
}

// NewMetrics tạo registry mới và đăng ký toàn bộ collector.
func NewMetrics(namespace string) *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,
		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Tổng số HTTP request theo method/path/status.",
		}, []string{"method", "path", "status"}),
		httpRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "Thời gian xử lý HTTP request (giây).",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "path"}),
		httpInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "http_requests_in_flight",
			Help:      "Số request đang được xử lý.",
		}),
		rateLimitFallback: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ratelimit_fallback_total",
			Help:      "Số lần rate limiter fail-open do backend (Redis) lỗi.",
		}),
	}

	reg.MustRegister(
		m.httpRequestsTotal,
		m.httpRequestDuration,
		m.httpInFlight,
		m.rateLimitFallback,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// ObserveHTTP ghi nhận một request đã hoàn tất.
func (m *Metrics) ObserveHTTP(method, path, status string, dur time.Duration) {
	m.httpRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.httpRequestDuration.WithLabelValues(method, path).Observe(dur.Seconds())
}

// IncInFlight/DecInFlight theo dõi số request đang xử lý.
func (m *Metrics) IncInFlight() { m.httpInFlight.Inc() }
func (m *Metrics) DecInFlight() { m.httpInFlight.Dec() }

// IncRateLimitFallback đếm số lần rate limiter fail-open.
func (m *Metrics) IncRateLimitFallback() { m.rateLimitFallback.Inc() }
