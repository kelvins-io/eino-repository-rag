package rag

import (
	"context"
	"fmt"
	"log"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

// redisBM25 基于 RediSearch TEXT 字段的 BM25 稀疏召回。
// - redis 向量后端：复用向量索引上的 content/title TEXT
// - milvus 后端：维护独立 sidecar 索引，与向量 Store/Delete 同步
type redisBM25 struct {
	rdb       *redis.Client
	indexName string
	keyPrefix string
	// reuseVectorIndex 为 true 时不双写，直接搜向量索引
	reuseVectorIndex bool
}

func newRedisBM25(ctx context.Context, cfg *config.Config, rdb *redis.Client) (*redisBM25, error) {
	reuse := strings.EqualFold(strings.TrimSpace(cfg.VectorIndex.Provider), config.VectorIndexRedis) ||
		strings.TrimSpace(cfg.VectorIndex.Provider) == ""

	b := &redisBM25{
		rdb:              rdb,
		reuseVectorIndex: reuse,
	}
	if reuse {
		b.indexName = cfg.Redis.IndexName
		b.keyPrefix = cfg.Redis.KeyPrefix
		return b, nil
	}

	b.indexName = cfg.RAG.BM25IndexName
	b.keyPrefix = cfg.RAG.BM25KeyPrefix
	if err := b.ensureSidecarIndex(ctx); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *redisBM25) ensureSidecarIndex(ctx context.Context) error {
	exists, err := b.rdb.Do(ctx, "FT.INFO", b.indexName).Result()
	if err == nil && exists != nil {
		log.Printf("[rag] redis bm25 sidecar index %s already exists", b.indexName)
		return nil
	}
	_, err = b.rdb.Do(ctx,
		"FT.CREATE", b.indexName,
		"ON", "HASH",
		"PREFIX", "1", b.keyPrefix,
		"SCHEMA",
		"content", "TEXT",
		"title", "TEXT",
		"doc_id", "TAG",
		"user_id", "TAG",
		"kb_id", "TAG",
		"directory_id", "TAG",
	).Result()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "index already exists") {
			return nil
		}
		return fmt.Errorf("create redis bm25 index: %w", err)
	}
	log.Printf("[rag] created redis bm25 sidecar index %s", b.indexName)
	return nil
}

// Upsert 将 chunk 写入 BM25 sidecar（reuse 向量索引时跳过）
func (b *redisBM25) Upsert(ctx context.Context, docs []*schema.Document) error {
	if b == nil || b.reuseVectorIndex || len(docs) == 0 {
		return nil
	}
	pipe := b.rdb.Pipeline()
	for _, d := range docs {
		if d == nil || d.ID == "" {
			continue
		}
		key := b.keyPrefix + d.ID
		pipe.HSet(ctx, key, map[string]interface{}{
			"content":      d.Content,
			"title":        metaString(d.MetaData, "title"),
			"doc_id":       metaString(d.MetaData, "doc_id"),
			"user_id":      metaString(d.MetaData, "user_id"),
			"kb_id":        metaString(d.MetaData, "kb_id"),
			"directory_id": metaString(d.MetaData, "directory_id"),
		})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("bm25 sidecar upsert: %w", err)
	}
	return nil
}

// DeleteByDocID 删除 sidecar 中某文档全部 chunk
func (b *redisBM25) DeleteByDocID(ctx context.Context, docID string) error {
	if b == nil || b.reuseVectorIndex {
		return nil
	}
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return nil
	}
	filter := fmt.Sprintf("@doc_id:{%s}", escapeRedisTag(docID))
	const pageSize = 1000
	for {
		raw, err := b.rdb.Do(ctx,
			"FT.SEARCH", b.indexName,
			filter,
			"NOCONTENT",
			"LIMIT", 0, pageSize,
		).Result()
		if err != nil {
			low := strings.ToLower(err.Error())
			if strings.Contains(low, "unknown index name") || strings.Contains(low, "no such index") {
				return nil
			}
			return fmt.Errorf("bm25 search by doc_id=%s: %w", docID, err)
		}
		keys, _, err := parseFTSearchKeys(raw)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		if err := b.rdb.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("bm25 delete doc_id=%s: %w", docID, err)
		}
	}
}

// Search BM25/全文检索，按相关度返回 topK
func (b *redisBM25) Search(ctx context.Context, query string, filter *RetrieveFilter, topK int) ([]*schema.Document, error) {
	if b == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = 10
	}
	textQ := buildRedisTextQuery(query)
	if textQ == "" {
		return nil, nil
	}
	filterQ := buildRedisFilter(filter)
	var searchQ string
	switch {
	case filterQ != "" && textQ != "":
		searchQ = filterQ + " " + textQ
	case textQ != "":
		searchQ = textQ
	default:
		return nil, nil
	}

	raw, err := b.rdb.Do(ctx,
		"FT.SEARCH", b.indexName,
		searchQ,
		"RETURN", 6, "content", "doc_id", "user_id", "kb_id", "directory_id", "title",
		"WITHSCORES",
		"LIMIT", 0, topK,
	).Result()
	if err != nil {
		return nil, fmt.Errorf("bm25 FT.SEARCH: %w", err)
	}
	docs, err := parseFTSearchDocs(raw)
	if err != nil {
		return nil, err
	}
	if !b.reuseVectorIndex && b.keyPrefix != "" {
		for _, d := range docs {
			d.ID = strings.TrimPrefix(d.ID, b.keyPrefix)
		}
	}
	return docs, nil
}

// buildRedisTextQuery 将用户查询转为 RediSearch 文本子句（OR 宽松召回）
func buildRedisTextQuery(query string) string {
	tokens := tokenizeQuery(query)
	if len(tokens) == 0 {
		return ""
	}
	escaped := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		t = escapeRedisTextToken(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		escaped = append(escaped, t)
	}
	if len(escaped) == 0 {
		return ""
	}
	return fmt.Sprintf("@content|title:(%s)", strings.Join(escaped, " | "))
}

func tokenizeQuery(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	var (
		tokens []string
		buf    strings.Builder
		cjk    []rune
	)
	flushLatin := func() {
		if buf.Len() == 0 {
			return
		}
		tok := strings.ToLower(buf.String())
		buf.Reset()
		if len([]rune(tok)) >= 1 {
			tokens = append(tokens, tok)
		}
	}
	flushCJK := func() {
		if len(cjk) == 0 {
			return
		}
		if len(cjk) == 1 {
			tokens = append(tokens, string(cjk[0]))
		} else {
			tokens = append(tokens, string(cjk))
			for i := 0; i+1 < len(cjk); i++ {
				tokens = append(tokens, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}

	for _, r := range query {
		switch {
		case unicode.In(r, unicode.Han):
			flushLatin()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			buf.WriteRune(r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return tokens
}

func escapeRedisTextToken(tok string) string {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return ""
	}
	specials := `,.<>{}[]"':;!@#$%^&*()-+=~|`
	var b strings.Builder
	for _, r := range tok {
		if strings.ContainsRune(specials, r) || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// parseFTSearchDocs 解析 FT.SEARCH WITHSCORES 返回：[total, key, score, [field, value, ...], ...]
func parseFTSearchDocs(raw any) ([]*schema.Document, error) {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil, fmt.Errorf("unexpected FT.SEARCH result type %T", raw)
	}
	docs := make([]*schema.Document, 0)
	i := 1
	for i < len(arr) {
		key, _ := anyToString(arr[i])
		i++
		score := 0.0
		if i < len(arr) {
			// WITHSCORES: next element is score (string/number)
			if s, ok := asFloat(arr[i]); ok {
				score = s
				i++
			}
		}
		fields := map[string]string{}
		if i < len(arr) {
			if fieldArr, ok := arr[i].([]any); ok {
				for j := 0; j+1 < len(fieldArr); j += 2 {
					fk, _ := anyToString(fieldArr[j])
					fv, _ := anyToString(fieldArr[j+1])
					fields[fk] = fv
				}
				i++
			}
		}
		if key == "" {
			continue
		}
		doc := &schema.Document{
			ID:      key,
			Content: fields["content"],
			MetaData: map[string]any{
				"doc_id":       fields["doc_id"],
				"user_id":      fields["user_id"],
				"kb_id":        fields["kb_id"],
				"directory_id": fields["directory_id"],
				"title":        fields["title"],
				"channel":      "bm25",
			},
		}
		doc.WithScore(score)
		docs = append(docs, doc)
	}
	return docs, nil
}

func anyToString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case []byte:
		return string(t), true
	default:
		if v == nil {
			return "", false
		}
		return fmt.Sprintf("%v", v), true
	}
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int64:
		return float64(t), true
	case int:
		return float64(t), true
	case string:
		var f float64
		_, err := fmt.Sscanf(t, "%f", &f)
		return f, err == nil
	case []byte:
		var f float64
		_, err := fmt.Sscanf(string(t), "%f", &f)
		return f, err == nil
	default:
		return 0, false
	}
}
