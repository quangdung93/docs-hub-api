package http

import "github.com/gin-gonic/gin"

// Register gắn các route của module user vào router group nội bộ.
// Middleware xác thực/RBAC do bootstrap áp ở cấp group, không phải ở đây.
func Register(rg *gin.RouterGroup, h *Handler) {
	users := rg.Group("/users")
	{
		users.POST("", h.Create)
		users.GET("", h.List)
		users.GET("/check-email", h.CheckEmail)
		users.GET("/:id", h.GetByID)
		users.PUT("/:id", h.Update)
		users.PATCH("/:id/status", h.UpdateStatus)
		users.DELETE("/:id", h.Delete)
	}
}
