package http

import "github.com/gin-gonic/gin"

func Register(rg *gin.RouterGroup, h *Handler) {
	p := rg.Group("/projects/:project_id/documents")
	p.POST("/uploads", h.Upload)
	p.POST("/uploads/presign", h.Presign)
	p.POST("/uploads/:upload_id/complete", h.Complete)
	p.GET("", h.List)
	p.GET("/:id", h.Detail)
	p.PATCH("/:id", h.Update)
	p.DELETE("/:id", h.Delete)
	p.POST("/:id/revisions", h.RevisionUpload)
	p.GET("/:id/revisions/:revision_id/status", h.Status)
	p.POST("/:id/revisions/:revision_id/retry", h.Retry)
	p.GET("/:id/revisions/:revision_id/view", h.View)
	p.GET("/:id/revisions/:revision_id/download", h.Download)
}
