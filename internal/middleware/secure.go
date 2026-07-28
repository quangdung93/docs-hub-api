package middleware

import "github.com/gin-gonic/gin"

// SecureHeaders gắn các security header phổ biến để giảm rủi ro XSS, clickjacking,
// MIME sniffing. HSTS chỉ nên có ý nghĩa khi chạy sau TLS (production).
func SecureHeaders(enableHSTS bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("X-XSS-Protection", "0") // dựa vào CSP thay vì bộ lọc XSS cũ của trình duyệt
		if enableHSTS {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		// Gin không tự set Server header; đảm bảo không lộ thông tin phiên bản.
		h.Del("X-Powered-By")

		c.Next()
	}
}
