package http

import "github.com/gin-gonic/gin"

func Register(internal *gin.RouterGroup, handler *Handler) {
	internal.POST("/projects", handler.Create)
	versions := internal.Group("/projects/:project_id/versions")
	versions.POST("", handler.CreateVersion)
	versions.GET("", handler.ListVersions)
}
