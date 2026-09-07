package http

import "github.com/gin-gonic/gin"

func Register(rg *gin.RouterGroup, h *Handler) {
	p := rg.Group("/projects/:id/reports")
	p.POST("", h.Generate)
	p.GET("/history", h.History)
}
