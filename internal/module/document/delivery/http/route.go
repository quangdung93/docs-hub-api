package http

import "github.com/gin-gonic/gin"

func Register(rg *gin.RouterGroup, h *Handler) {
	p := rg.Group("/projects/:id/documents")
	p.POST("/uploads", h.Upload)
	p.POST("/uploads/presign", h.Presign)
	p.POST("/uploads/:upload_id/complete", h.Complete)
	p.GET("", h.List)
	p.GET("/:document_id", h.Detail)
	p.PATCH("/:document_id", h.Update)
	p.DELETE("/:document_id", h.Delete)
	p.POST("/:document_id/revisions", h.RevisionUpload)
	p.GET("/:document_id/revisions/:revision_id/status", h.Status)
	p.POST("/:document_id/revisions/:revision_id/retry", h.Retry)
	p.GET("/:document_id/revisions/:revision_id/view", h.View)
	p.GET("/:document_id/revisions/:revision_id/download", h.Download)
}
