package http

import "github.com/gin-gonic/gin"

func Register(rg *gin.RouterGroup, handler *Handler) {
	rg.POST("/projects/:project_id/retrieval", handler.Retrieve)
}
