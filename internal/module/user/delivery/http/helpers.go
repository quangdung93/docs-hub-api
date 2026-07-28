package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung393/docs-hub-api/internal/common/apperr"
	"github.com/quangdung393/docs-hub-api/internal/common/validatorx"
	"github.com/quangdung393/docs-hub-api/internal/module/user/domain"
)

// bindBody bind JSON body; lỗi -> REQ_400 kèm details. Trả false nếu lỗi.
func bindBody(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		_ = c.Error(apperr.BadRequest("Dữ liệu không hợp lệ").WithDetails(validatorx.ToDetails(err)))
		return false
	}
	return true
}

// parseID đọc và parse UUID từ path param "id". Lỗi -> REQ_400. Trả ok=false nếu lỗi.
func parseID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(apperr.BadRequest("ID không hợp lệ").
			WithDetails(map[string]string{"id": "phải là UUID hợp lệ"}))
		return uuid.Nil, false
	}
	return id, true
}

// statusFromString ép chuỗi đã validate thành domain.Status.
func statusFromString(s string) domain.Status {
	return domain.Status(s)
}
