package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout đặt deadline cho context của request. Khi hết hạn, ctx.Done() được
// kích hoạt; các thao tác DB/Redis/HTTP nhận context sẽ dừng và trả về
// context.DeadlineExceeded, được ErrorHandler map thành REQ_TIMEOUT (408).
//
// Cách này (chỉ cancel context, để ErrorHandler ghi response) tránh việc
// http.TimeoutHandler ghi đè response gây double-write với gin.
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d <= 0 {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		// Nếu deadline hết mà handler chưa ghi gì, đẩy lỗi timeout để ErrorHandler xử lý.
		if ctx.Err() != nil && !c.Writer.Written() && len(c.Errors) == 0 {
			_ = c.Error(context.DeadlineExceeded)
		}
	}
}
