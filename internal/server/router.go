package server

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/kelvins-io/eino-repository-rag/internal/auth"
	"github.com/kelvins-io/eino-repository-rag/internal/handler"
	"github.com/kelvins-io/eino-repository-rag/internal/logger"
)

func NewRouter(mode string, kh *handler.KnowledgeHandler, ah *handler.AuthHandler, tm *auth.TokenManager, agentEnabled bool) *gin.Engine {
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

	r.GET("/health", kh.Health)

	api := r.Group("/api/v1")
	{
		authGroup := api.Group("/auth")
		{
			authGroup.POST("/register", ah.Register)
			authGroup.POST("/login", ah.Login)
		}

		protected := api.Group("")
		protected.Use(auth.Middleware(tm), ah.RequireLoginEnabled())
		{
			// 仅 default 租户的 admin 可创建租户
			protected.POST("/tenants", ah.CreateTenant)
			protected.GET("/users", ah.ListUsers)
			protected.PUT("/users/login-enabled", ah.SetUserLoginEnabled)
			protected.GET("/auth/me", ah.Me)

			kbs := protected.Group("/knowledge-bases")
			{
				kbs.POST("", kh.CreateKnowledgeBase)
				kbs.GET("", kh.ListKnowledgeBases)
				kbs.GET("/:id", kh.GetKnowledgeBase)
				kbs.PUT("/:id", kh.UpdateKnowledgeBase)
				kbs.DELETE("/:id", kh.DeleteKnowledgeBase)
				kbs.POST("/:id/directories", kh.CreateDirectory)
				kbs.GET("/:id/directories", kh.ListDirectoryTree)
			}

			dirs := protected.Group("/directories")
			{
				dirs.PUT("/:id", kh.UpdateDirectory)
				dirs.DELETE("/:id", kh.DeleteDirectory)
			}

			docs := protected.Group("/documents")
			{
				docs.POST("/import", kh.ImportDocument)
				docs.POST("/reindex", kh.ReindexDocuments)
				docs.POST("/delete", kh.DeleteDocuments)
				docs.GET("", kh.ListDocuments)
				docs.GET("/:id", kh.GetDocument)
				docs.DELETE("/:id", kh.DeleteDocument)
			}

			protected.GET("/system/upload-limits", kh.UploadLimits)

			chat := protected.Group("/chat")
			{
				chat.POST("/query", kh.Query)
				if agentEnabled {
					chat.POST("/agent", kh.AgentQuery)
				}
				chat.POST("/transcribe", kh.TranscribeSpeech)
				chat.POST("/speech", kh.SynthesizeSpeech)
				chat.GET("/history", kh.History)
				chat.GET("/sessions", kh.ListSessions)
			}
		}
	}
	return r
}
