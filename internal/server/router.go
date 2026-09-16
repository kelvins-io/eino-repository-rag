package server

import (
	"github.com/gin-gonic/gin"

	"github.com/kelvins-io/eino-repository-rag/internal/handler"
)

func NewRouter(mode string, h *handler.KnowledgeHandler) *gin.Engine {
	gin.SetMode(mode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/health", h.Health)

	api := r.Group("/api/v1")
	{
		kbs := api.Group("/knowledge-bases")
		{
			kbs.POST("", h.CreateKnowledgeBase)
			kbs.GET("", h.ListKnowledgeBases)
			kbs.GET("/:id", h.GetKnowledgeBase)
			kbs.PUT("/:id", h.UpdateKnowledgeBase)
			kbs.DELETE("/:id", h.DeleteKnowledgeBase)
			kbs.POST("/:id/directories", h.CreateDirectory)
			kbs.GET("/:id/directories", h.ListDirectoryTree)
		}

		dirs := api.Group("/directories")
		{
			dirs.PUT("/:id", h.UpdateDirectory)
			dirs.DELETE("/:id", h.DeleteDirectory)
		}

		docs := api.Group("/documents")
		{
			docs.POST("/import", h.ImportDocument)
			docs.POST("/reindex", h.ReindexDocuments)
			docs.GET("", h.ListDocuments)
			docs.GET("/:id", h.GetDocument)
		}

		chat := api.Group("/chat")
		{
			chat.POST("/query", h.Query)
			chat.GET("/history", h.History)
		}
	}
	return r
}
