package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/kelvins-io/eino-repository-rag/internal/asr"
	"github.com/kelvins-io/eino-repository-rag/internal/auth"
	"github.com/kelvins-io/eino-repository-rag/internal/config"
	"github.com/kelvins-io/eino-repository-rag/internal/handler"
	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	"github.com/kelvins-io/eino-repository-rag/internal/memory"
	"github.com/kelvins-io/eino-repository-rag/internal/rag"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
	"github.com/kelvins-io/eino-repository-rag/internal/server"
	"github.com/kelvins-io/eino-repository-rag/internal/service"
	"github.com/kelvins-io/eino-repository-rag/internal/tts"
)

func main() {
	defer logger.Sync()

	configPath := flag.String("config", "configs/config.yaml", "config file path")
	flag.Parse()

	_ = godotenv.Load()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.L().Fatal("load config failed", zap.Error(err))
	}

	if err := logger.Init(logger.Config{
		Level:            cfg.Log.Level,
		Encoding:         cfg.Log.Encoding,
		OutputPaths:      cfg.Log.OutputPaths,
		ErrorOutputPaths: cfg.Log.ErrorOutputPaths,
	}); err != nil {
		logger.L().Fatal("init logger failed", zap.Error(err))
	}

	if err := os.MkdirAll(cfg.RAG.UploadDir, 0o755); err != nil {
		logger.L().Fatal("create upload dir failed", zap.Error(err))
	}

	db, err := repository.NewPostgres(cfg.Postgres)
	if err != nil {
		logger.L().Fatal("connect postgres failed", zap.Error(err))
	}

	tenantRepo := repository.NewTenantRepo(db)
	defTenant, err := tenantRepo.EnsureDefault()
	if err != nil {
		logger.L().Fatal("ensure default tenant failed", zap.Error(err))
	}
	userRepo := repository.NewUserRepo(db)
	tokenMgr := auth.NewTokenManager(cfg.JWT)
	authSvc := service.NewAuthService(tenantRepo, userRepo, tokenMgr)
	if err := authSvc.EnsureTenantAdmin(defTenant); err != nil {
		logger.L().Fatal("ensure default tenant admin failed", zap.Error(err))
	}
	authHandler := handler.NewAuthHandler(authSvc)

	// Redis：Protocol=2 + UnstableResp3 是向量检索前置条件
	rdb := config.RedisClient(cfg.Redis)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Redis.PingTimeout())
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.L().Fatal("redis ping failed", zap.Error(err))
	}

	docRepo := repository.NewDocumentRepo(db)
	kbRepo := repository.NewKnowledgeBaseRepo(db)
	dirRepo := repository.NewDirectoryRepo(db)
	convRepo := repository.NewConversationRepo(db)
	msgRepo := repository.NewMessageRepo(db)

	memMgr := memory.NewManager(rdb, msgRepo, convRepo, cfg.Memory)

	pipeline, err := rag.NewPipeline(context.Background(), cfg, rdb, docRepo, memMgr)
	if err != nil {
		logger.L().Fatal("init rag pipeline failed", zap.Error(err))
	}

	queueCtx, queueCancel := context.WithCancel(context.Background())
	defer queueCancel()
	if err := pipeline.StartIndexQueue(queueCtx); err != nil {
		logger.L().Fatal("start index queue failed", zap.Error(err))
	}

	svc := service.NewKnowledgeService(docRepo, kbRepo, dirRepo, msgRepo, memMgr, pipeline)
	kh := handler.NewKnowledgeHandler(svc, cfg.RAG)
	if cfg.ASR.Enabled {
		kh.WithASR(asr.NewClient(cfg.ASR), cfg.ASR.MaxAudioBytes())
	}
	if cfg.TTS.Enabled {
		kh.WithTTS(tts.NewClient(cfg.TTS))
	}
	router := server.NewRouter(cfg.Server.Mode, kh, authHandler, tokenMgr, cfg.Agent.Enabled)
	router.MaxMultipartMemory = cfg.RAG.MaxUploadFileSizeBytes()

	go func() {
		logger.L().Info("eino knowledge base RAG listening", zap.String("addr", cfg.Server.Addr))
		if err := router.Run(cfg.Server.Addr); err != nil {
			logger.L().Fatal("server stopped", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.L().Info("shutting down...")
	queueCancel()
	pipeline.StopIndexQueue()
	_ = rdb.Close()
}
