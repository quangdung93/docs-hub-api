package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit giới hạn kích thước body request bằng http.MaxBytesReader, chống
// payload bomb. Khi vượt giới hạn, việc đọc body sẽ lỗi và handler bind sẽ trả
// REQ_400 (qua ErrorHandler).
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
