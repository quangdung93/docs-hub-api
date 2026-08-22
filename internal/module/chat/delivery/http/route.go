package http

import "github.com/gin-gonic/gin"

func Register(rg *gin.RouterGroup, handler *Handler) {
	rg.POST("/projects/:id/conversations", handler.Create)
	rg.GET("/projects/:id/conversations", handler.List)
	rg.GET("/projects/:id/conversations/:conversation_id", handler.Get)
	rg.POST("/projects/:id/conversations/:conversation_id/messages", handler.Ask)
}
