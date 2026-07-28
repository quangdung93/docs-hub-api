package response

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
)

// buildMeta dựng Meta từ context của request (request_id, trace_id) + timestamp.
func buildMeta(c *gin.Context) Meta {
	ctx := c.Request.Context()
	return Meta{
		RequestID: contextx.RequestID(ctx),
		TraceID:   contextx.TraceID(ctx),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// OK ghi phản hồi thành công (HTTP 200) với data tùy ý.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, newSuccess(data, buildMeta(c)))
}

// Created ghi phản hồi tạo mới thành công (HTTP 201).
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, newSuccess(data, buildMeta(c)))
}

// NoContent ghi phản hồi 204 (không có body theo chuẩn HTTP).
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// OKPaged ghi phản hồi danh sách kèm metadata phân trang (HTTP 200).
func OKPaged(c *gin.Context, items any, page pagination.Meta) {
	meta := buildMeta(c)
	meta.Pagination = &page
	c.JSON(http.StatusOK, newSuccess(items, meta))
}

// Error ghi phản hồi lỗi với HTTP status chỉ định.
// Hàm này do middleware errorhandler gọi — code khác KHÔNG nên gọi trực tiếp.
func Error(c *gin.Context, status int, body ErrorBody) {
	c.JSON(status, newError(&body, buildMeta(c)))
}
