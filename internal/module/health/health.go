// Package health cung cấp liveness và readiness probe.
//
//   - Liveness  : tiến trình còn sống? (luôn 200 nếu HTTP handler chạy được)
//   - Readiness : mọi dependency (PostgreSQL, Redis, ...) đã sẵn sàng chưa?
//
// Readiness chạy các checker song song với timeout, trả 200 nếu tất cả OK,
// 503 nếu có bất kỳ dependency nào lỗi.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/quangdung393/docs-hub-api/internal/common/port"
)

const checkTimeout = 2 * time.Second

// Checker gom danh sách dependency cần kiểm tra cho readiness.
type Checker struct {
	deps []port.HealthChecker
}

// New tạo Checker từ danh sách dependency.
func New(deps ...port.HealthChecker) *Checker {
	return &Checker{deps: deps}
}

// depResult là kết quả kiểm tra 1 dependency.
type depResult struct {
	Status string `json:"status"`          // "up" | "down"
	Error  string `json:"error,omitempty"` // mô tả lỗi nếu down
}

// LivenessHandler luôn trả 200 — chỉ khẳng định tiến trình đang chạy.
func (c *Checker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// ReadinessHandler kiểm tra toàn bộ dependency song song.
func (c *Checker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
		defer cancel()

		results := make(map[string]depResult, len(c.deps))
		statuses := make(chan struct {
			name string
			res  depResult
		}, len(c.deps))

		for _, d := range c.deps {
			go func(dep port.HealthChecker) {
				res := depResult{Status: "up"}
				if err := dep.Check(ctx); err != nil {
					res = depResult{Status: "down", Error: err.Error()}
				}
				statuses <- struct {
					name string
					res  depResult
				}{dep.Name(), res}
			}(d)
		}

		allUp := true
		for range c.deps {
			s := <-statuses
			results[s.name] = s.res
			if s.res.Status != "up" {
				allUp = false
			}
		}

		code := http.StatusOK
		if !allUp {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]any{
			"status":       statusText(allUp),
			"dependencies": results,
		})
	}
}

func statusText(allUp bool) string {
	if allUp {
		return "ready"
	}
	return "not_ready"
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
