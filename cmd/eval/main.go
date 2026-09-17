package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
	"github.com/kelvins-io/eino-repository-rag/internal/memory"
	"github.com/kelvins-io/eino-repository-rag/internal/rag"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "config file path")
	goldenPath := flag.String("golden", "examples/eval/golden.jsonl", "golden JSONL path")
	outPath := flag.String("out", "", "write full report JSON to file (optional)")
	k := flag.Int("k", 0, "Hit@K / Recall@K；0 表示使用 rag.top_k")
	timeoutSec := flag.Int("timeout", 120, "per-case retrieve timeout seconds")
	flag.Parse()

	_ = godotenv.Load()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}
	topK := *k
	if topK <= 0 {
		topK = cfg.RAG.TopK
		if topK <= 0 {
			topK = 5
		}
	}

	cases, err := rag.LoadGoldenJSONL(*goldenPath)
	if err != nil {
		fatalf("load golden: %v", err)
	}

	db, err := repository.NewPostgres(cfg.Postgres)
	if err != nil {
		fatalf("postgres: %v", err)
	}
	rdb := config.RedisClient(cfg.Redis)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Redis.PingTimeout())
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fatalf("redis ping: %v", err)
	}

	docRepo := repository.NewDocumentRepo(db)
	convRepo := repository.NewConversationRepo(db)
	msgRepo := repository.NewMessageRepo(db)
	memMgr := memory.NewManager(rdb, msgRepo, convRepo, cfg.Memory)

	pipeline, err := rag.NewPipeline(context.Background(), cfg, rdb, docRepo, memMgr)
	if err != nil {
		fatalf("pipeline: %v", err)
	}
	defer func() { _ = rdb.Close() }()

	scores := make([]rag.CaseScore, 0, len(cases))
	perTimeout := time.Duration(*timeoutSec) * time.Second

	fmt.Printf("Golden Eval: cases=%d k=%d expand=%v hybrid=%v rerank=%v\n",
		len(cases), topK, cfg.RAG.QueryExpandEnabled, cfg.RAG.HybridEnabled, cfg.Rerank.Enabled)

	for i, c := range cases {
		fmt.Printf("[%d/%d] %s … ", i+1, len(cases), c.ID)
		cctx, ccancel := context.WithTimeout(context.Background(), perTimeout)
		filter := rag.FilterFromGolden(c)
		docs, err := pipeline.Retrieve(cctx, c.Query, filter)
		ccancel()
		if err != nil {
			cs := rag.CaseScore{ID: c.ID, Query: c.Query, Error: err.Error()}
			scores = append(scores, cs)
			fmt.Printf("ERR %v\n", err)
			continue
		}
		cs := rag.ScoreRetrieval(c, docs, topK)
		scores = append(scores, cs)
		fmt.Printf("hit=%v rank=%d recall=%.2f rr=%.2f retrieved=%d\n",
			cs.HitAtK, cs.FirstRank, cs.RecallAtK, cs.RR, cs.Retrieved)
	}

	report := rag.AggregateScores(scores, topK)
	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Printf("scored=%d/%d  Hit@%d=%.4f  Recall@%d=%.4f  MRR=%.4f\n",
		report.Scored, report.Cases, report.K, report.HitAtK, report.K, report.RecallAtK, report.MRR)

	if *outPath != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fatalf("marshal report: %v", err)
		}
		if err := os.WriteFile(*outPath, data, 0o644); err != nil {
			fatalf("write report: %v", err)
		}
		fmt.Printf("wrote %s\n", *outPath)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
