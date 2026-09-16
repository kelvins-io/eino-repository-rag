package server

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/kelvins-io/eino-repository-rag/internal/handler"
	"github.com/kelvins-io/eino-repository-rag/internal/logger"
)

func NewRouter(mode string, h *handler.KnowledgeHandler) *gin.Engine {
	gin.SetMode(mode)
	r := gin.New()
	r.Use(logger.GinLogger(), logger.GinRecovery(true))
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

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
			docs.POST("/delete", h.DeleteDocuments)
			docs.GET("", h.ListDocuments)
			docs.GET("/:id", h.GetDocument)
			docs.DELETE("/:id", h.DeleteDocument)
		}

		chat := api.Group("/chat")
		{
			chat.POST("/query", h.Query)
			chat.GET("/history", h.History)
		}
	}
	return r
}
