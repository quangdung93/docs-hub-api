package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/response"
	"github.com/quangdung93/docs-hub-api/pkg/logger"
)

// Recovery bắt panic, log kèm stack trace, rồi tự ghi envelope SYS_500.
//
// Đặt NGOÀI CÙNG chuỗi middleware để bắt panic của mọi thứ bên trong. Recovery
// PHẢI tự ghi response (không đẩy qua ErrorHandler) vì panic làm ngăn xếp bung
// qua ErrorHandler, bỏ qua đoạn code ghi response sau c.Next() của nó.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}

			log := logger.FromContext(c.Request.Context())
			log.Error("panic được recover",
				zap.Any("panic", r),
				zap.ByteString("stack", debug.Stack()),
			)

			if c.Writer.Written() {
				// Response đã ghi dở -> chỉ có thể abort, không ghi đè được.
				c.Abort()
				return
			}

			response.Error(c, http.StatusInternalServerError, response.ErrorBody{
				Code:      errcode.Sys500,
				Message:   "Đã có lỗi hệ thống. Vui lòng thử lại sau.",
				Retryable: false,
			})
			c.Abort()
		}()
		c.Next()
	}
}
