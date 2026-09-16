package rag

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

// RetrieveFilter 向量检索过滤条件（知识库 / 目录）
type RetrieveFilter struct {
	KnowledgeBaseID string
	DirectoryIDs    []string // 为空表示不按目录过滤；多个 ID 为 OR
}

// VectorStore 向量索引抽象：支持 Redis / Milvus Lite 等后端切换
type VectorStore interface {
	Store(ctx context.Context, docs []*schema.Document) error
	Retrieve(ctx context.Context, query string, filter *RetrieveFilter) ([]*schema.Document, error)
	// DeleteByDocID 按文档 ID 删除其全部向量 chunk（doc 不存在时视为成功）
	DeleteByDocID(ctx context.Context, docID string) error
}

func newVectorStore(ctx context.Context, cfg *config.Config, rdb *redis.Client, emb embedding.Embedder) (VectorStore, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.VectorIndex.Provider)) {
	case "", config.VectorIndexRedis:
		return newRedisVectorStore(ctx, cfg, rdb, emb)
	case config.VectorIndexMilvusLite, "milvus":
		return newMilvusLiteVectorStore(ctx, cfg, emb)
	default:
		return nil, fmt.Errorf("unsupported vector_index.provider %q (use redis or milvus_lite)", cfg.VectorIndex.Provider)
	}
}

func uintToMeta(id uint) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(id), 10)
}

func ptrUintToMeta(id *uint) string {
	if id == nil || *id == 0 {
		return "0"
	}
	return strconv.FormatUint(uint64(*id), 10)
}
