package http

import "github.com/gin-gonic/gin"

func Register(rg *gin.RouterGroup, handler *Handler) {
	rg.POST("/projects/:id/search", handler.Retrieve)
	// Giữ alias cũ để client đang tích hợp retrieval không bị gãy ngay.
	rg.POST("/projects/:id/retrieval", handler.Retrieve)
}
