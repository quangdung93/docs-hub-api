package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung393/docs-hub-api/internal/common/contextx"
)

// HeaderRequestID là tên header mang request id xuyên hệ thống.
const HeaderRequestID = "X-Request-ID"

// RequestID đọc X-Request-ID từ client (nếu có), hoặc sinh mới, rồi đưa vào
// context và trả lại qua response header. request_id là định danh 1 request,
// khác với trace_id (định danh 1 luồng xử lý xuyên nhiều service).
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}

		ctx := contextx.WithRequestID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Writer.Header().Set(HeaderRequestID, id)

		c.Next()
	}
}
