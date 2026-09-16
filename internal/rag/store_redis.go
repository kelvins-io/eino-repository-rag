package rag

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	redisindexer "github.com/cloudwego/eino-ext/components/indexer/redis"
	redisretriever "github.com/cloudwego/eino-ext/components/retriever/redis"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

type redisVectorStore struct {
	cfg      *config.Config
	rdb      *redis.Client
	embedder embedding.Embedder
}

func newRedisVectorStore(ctx context.Context, cfg *config.Config, rdb *redis.Client, emb embedding.Embedder) (*redisVectorStore, error) {
	s := &redisVectorStore{cfg: cfg, rdb: rdb, embedder: emb}
	if err := s.ensureIndex(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *redisVectorStore) ensureIndex(ctx context.Context) error {
	exists, err := s.rdb.Do(ctx, "FT.INFO", s.cfg.Redis.IndexName).Result()
	if err == nil && exists != nil {
		log.Printf("[rag] redis vector index %s already exists", s.cfg.Redis.IndexName)
		// 兼容旧索引：尝试补充分类字段
		_, _ = s.rdb.Do(ctx, "FT.ALTER", s.cfg.Redis.IndexName, "SCHEMA", "ADD", "kb_id", "TAG").Result()
		_, _ = s.rdb.Do(ctx, "FT.ALTER", s.cfg.Redis.IndexName, "SCHEMA", "ADD", "directory_id", "TAG").Result()
		return nil
	}

	_, err = s.rdb.Do(ctx,
		"FT.CREATE", s.cfg.Redis.IndexName,
		"ON", "HASH",
		"PREFIX", "1", s.cfg.Redis.KeyPrefix,
		"SCHEMA",
		"content", "TEXT",
		"doc_id", "TAG",
		"user_id", "TAG",
		"kb_id", "TAG",
		"directory_id", "TAG",
		"title", "TEXT",
		s.cfg.Redis.VectorField, "VECTOR", "HNSW", "6",
		"TYPE", "FLOAT32",
		"DIM", strconv.Itoa(s.cfg.Redis.VectorDim),
		"DISTANCE_METRIC", "COSINE",
	).Result()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "index already exists") {
			return nil
		}
		return fmt.Errorf("create redis vector index: %w", err)
	}
	log.Printf("[rag] created redis vector index %s", s.cfg.Redis.IndexName)
	return nil
}

func (s *redisVectorStore) Store(ctx context.Context, docs []*schema.Document) error {
	indexer, err := redisindexer.NewIndexer(ctx, &redisindexer.IndexerConfig{
		Client:    s.rdb,
		KeyPrefix: s.cfg.Redis.KeyPrefix,
		BatchSize: 10,
		Embedding: s.embedder,
		DocumentToHashes: func(ctx context.Context, d *schema.Document) (*redisindexer.Hashes, error) {
			return &redisindexer.Hashes{
				Key: d.ID,
				Field2Value: map[string]redisindexer.FieldValue{
					"content": {
						Value:    d.Content,
						EmbedKey: s.cfg.Redis.VectorField,
					},
					"doc_id":       {Value: metaString(d.MetaData, "doc_id")},
					"user_id":      {Value: metaString(d.MetaData, "user_id")},
					"kb_id":        {Value: metaString(d.MetaData, "kb_id")},
					"directory_id": {Value: metaString(d.MetaData, "directory_id")},
					"title":        {Value: metaString(d.MetaData, "title")},
				},
			}, nil
		},
	})
	if err != nil {
		return fmt.Errorf("create redis indexer: %w", err)
	}
	if _, err := indexer.Store(ctx, docs); err != nil {
		return fmt.Errorf("redis store vectors: %w", err)
	}
	return nil
}

// DeleteByDocID 通过 RediSearch TAG 查找并删除该文档的全部向量 hash
func (s *redisVectorStore) DeleteByDocID(ctx context.Context, docID string) error {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return fmt.Errorf("doc_id is required")
	}

	filter := fmt.Sprintf("@doc_id:{%s}", escapeRedisTag(docID))
	const pageSize = 1000
	// 删除后结果集会收缩，始终从 offset=0 拉取，直到清空
	for {
		raw, err := s.rdb.Do(ctx,
			"FT.SEARCH", s.cfg.Redis.IndexName,
			filter,
			"NOCONTENT",
			"LIMIT", 0, pageSize,
		).Result()
		if err != nil {
			// 索引不存在时视为无需清理
			if strings.Contains(strings.ToLower(err.Error()), "unknown index name") ||
				strings.Contains(strings.ToLower(err.Error()), "no such index") {
				return nil
			}
			return fmt.Errorf("redis search vectors by doc_id=%s: %w", docID, err)
		}

		keys, _, err := parseFTSearchKeys(raw)
		if err != nil {
			return fmt.Errorf("parse redis search result: %w", err)
		}
		if len(keys) == 0 {
			return nil
		}

		if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("redis delete vectors doc_id=%s: %w", docID, err)
		}
	}
}

// parseFTSearchKeys 解析 FT.SEARCH NOCONTENT 返回值：[total, key1, key2, ...]
func parseFTSearchKeys(raw any) ([]string, int, error) {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil, 0, fmt.Errorf("unexpected FT.SEARCH result type %T", raw)
	}
	total, err := toInt(arr[0])
	if err != nil {
		return nil, 0, err
	}
	keys := make([]string, 0, len(arr)-1)
	for _, item := range arr[1:] {
		switch v := item.(type) {
		case string:
			if v != "" {
				keys = append(keys, v)
			}
		case []byte:
			if len(v) > 0 {
				keys = append(keys, string(v))
			}
		}
	}
	return keys, total, nil
}

func toInt(v any) (int, error) {
	switch t := v.(type) {
	case int64:
		return int(t), nil
	case int:
		return t, nil
	case uint64:
		return int(t), nil
	case string:
		n, err := strconv.Atoi(t)
		return n, err
	case []byte:
		n, err := strconv.Atoi(string(t))
		return n, err
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func (s *redisVectorStore) Retrieve(ctx context.Context, query string, filter *RetrieveFilter) ([]*schema.Document, error) {
	retriever, err := redisretriever.NewRetriever(ctx, &redisretriever.RetrieverConfig{
		Client:       s.rdb,
		Index:        s.cfg.Redis.IndexName,
		VectorField:  s.cfg.Redis.VectorField,
		TopK:         s.cfg.RAG.TopK,
		Embedding:    s.embedder,
		ReturnFields: []string{"content", "doc_id", "user_id", "kb_id", "directory_id", "title", s.cfg.Redis.VectorField},
		DocumentConverter: func(ctx context.Context, doc redis.Document) (*schema.Document, error) {
			return &schema.Document{
				ID:      doc.ID,
				Content: doc.Fields["content"],
				MetaData: map[string]any{
					"doc_id":       doc.Fields["doc_id"],
					"user_id":      doc.Fields["user_id"],
					"kb_id":        doc.Fields["kb_id"],
					"directory_id": doc.Fields["directory_id"],
					"title":        doc.Fields["title"],
				},
			}, nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create redis retriever: %w", err)
	}

	filterQuery := buildRedisFilter(filter)
	if filterQuery != "" {
		docs, err := retriever.Retrieve(ctx, query, redisretriever.WithFilterQuery(filterQuery))
		if err != nil {
			return nil, fmt.Errorf("redis retrieve: %w", err)
		}
		return docs, nil
	}

	docs, err := retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("redis retrieve: %w", err)
	}
	return docs, nil
}

func buildRedisFilter(filter *RetrieveFilter) string {
	if filter == nil {
		return ""
	}
	var parts []string
	if filter.KnowledgeBaseID != "" {
		parts = append(parts, fmt.Sprintf("@kb_id:{%s}", escapeRedisTag(filter.KnowledgeBaseID)))
	}
	if len(filter.DirectoryIDs) > 0 {
		escaped := make([]string, 0, len(filter.DirectoryIDs))
		for _, id := range filter.DirectoryIDs {
			if id == "" {
				continue
			}
			escaped = append(escaped, escapeRedisTag(id))
		}
		if len(escaped) == 1 {
			parts = append(parts, fmt.Sprintf("@directory_id:{%s}", escaped[0]))
		} else if len(escaped) > 1 {
			parts = append(parts, fmt.Sprintf("@directory_id:{%s}", strings.Join(escaped, "|")))
		}
	}
	return strings.Join(parts, " ")
}

func escapeRedisTag(v string) string {
	specials := []string{",", ".", "<", ">", "{", "}", "[", "]", `"`, "'", ":", ";", "!", "@", "#", "$", "%", "^", "&", "*", "(", ")", "-", "+", "=", "~"}
	for _, s := range specials {
		v = strings.ReplaceAll(v, s, "\\"+s)
	}
	return v
}
