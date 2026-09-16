package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
	"github.com/kelvins-io/eino-repository-rag/internal/handler"
	"github.com/kelvins-io/eino-repository-rag/internal/memory"
	"github.com/kelvins-io/eino-repository-rag/internal/rag"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
	"github.com/kelvins-io/eino-repository-rag/internal/server"
	"github.com/kelvins-io/eino-repository-rag/internal/service"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "config file path")
	flag.Parse()

	_ = godotenv.Load()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := os.MkdirAll(cfg.RAG.UploadDir, 0o755); err != nil {
		log.Fatalf("create upload dir: %v", err)
	}

	db, err := repository.NewPostgres(cfg.Postgres)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}

	// Redis：Protocol=2 + UnstableResp3 是向量检索前置条件
	rdb := redis.NewClient(&redis.Options{
		Addr:           cfg.Redis.Addr,
		Password:       cfg.Redis.Password,
		DB:             cfg.Redis.DB,
		Protocol:       2,
		UnstableResp3:  true,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping: %v", err)
	}

	docRepo := repository.NewDocumentRepo(db)
	kbRepo := repository.NewKnowledgeBaseRepo(db)
	dirRepo := repository.NewDirectoryRepo(db)
	convRepo := repository.NewConversationRepo(db)
	msgRepo := repository.NewMessageRepo(db)

	memMgr := memory.NewManager(rdb, msgRepo, convRepo, cfg.Memory)

	pipeline, err := rag.NewPipeline(context.Background(), cfg, rdb, docRepo, memMgr)
	if err != nil {
		log.Fatalf("init rag pipeline: %v", err)
	}

	svc := service.NewKnowledgeService(docRepo, kbRepo, dirRepo, msgRepo, memMgr, pipeline)
	h := handler.NewKnowledgeHandler(svc)
	router := server.NewRouter(cfg.Server.Mode, h)

	go func() {
		log.Printf("eino knowledge base RAG listening on %s", cfg.Server.Addr)
		if err := router.Run(cfg.Server.Addr); err != nil {
			log.Fatalf("server stopped: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	_ = rdb.Close()
}
