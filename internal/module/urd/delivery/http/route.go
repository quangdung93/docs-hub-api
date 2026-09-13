package http

import "github.com/gin-gonic/gin"

func Register(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/projects/:id/documents/urd-summary", h.ListSummaries)
	g := rg.Group("/projects/:id/documents/:document_id/urd")
	g.POST("/analyze", h.Analyze)
	g.GET("/analyses/:analysis_id", h.GetAnalysis)
	g.POST("/analyses/:analysis_id/cases/:case_id/image", h.UploadCaseImage)
	g.POST("/analyses/:analysis_id/resolutions", h.SubmitResolutions)
}
