package rag

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/embedding"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
	"github.com/kelvins-io/eino-repository-rag/internal/memory"
	dbmodel "github.com/kelvins-io/eino-repository-rag/internal/model"
	docparser "github.com/kelvins-io/eino-repository-rag/internal/rag/parser"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

// Pipeline 基于 Eino 的企业知识库 RAG 流水线
type Pipeline struct {
	cfg      *config.Config
	embedder embedding.Embedder
	chat     einomodel.BaseChatModel
	splitter document.Transformer
	store    VectorStore
	bm25     *redisBM25
	reranker Reranker
	docRepo  *repository.DocumentRepo
	mem      *memory.Manager
}

func NewPipeline(
	ctx context.Context,
	cfg *config.Config,
	rdb *redis.Client,
	docRepo *repository.DocumentRepo,
	mem *memory.Manager,
) (*Pipeline, error) {
	dim := cfg.Embedding.Dimensions
	embCfg := &openai.EmbeddingConfig{
		APIKey:  cfg.Embedding.APIKey,
		Model:   cfg.Embedding.Model,
		Timeout: 60 * time.Second,
	}
	if cfg.Embedding.BaseURL != "" {
		embCfg.BaseURL = cfg.Embedding.BaseURL
	}
	if dim > 0 {
		embCfg.Dimensions = &dim
	}
	emb, err := openai.NewEmbedder(ctx, embCfg)
	if err != nil {
		return nil, fmt.Errorf("create embedder: %w", err)
	}

	chat, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:      cfg.DeepSeek.APIKey,
		Model:       cfg.DeepSeek.Model,
		BaseURL:     cfg.DeepSeek.BaseURL,
		MaxTokens:   cfg.DeepSeek.MaxTokens,
		Temperature: cfg.DeepSeek.Temperature,
		Timeout:     120 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("create deepseek chat model: %w", err)
	}

	splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize:   cfg.RAG.ChunkSize,
		OverlapSize: cfg.RAG.OverlapSize,
	})
	if err != nil {
		return nil, fmt.Errorf("create splitter: %w", err)
	}

	store, err := newVectorStore(ctx, cfg, rdb, emb)
	if err != nil {
		return nil, fmt.Errorf("init vector store: %w", err)
	}

	var bm25 *redisBM25
	if cfg.RAG.HybridEnabled {
		bm25, err = newRedisBM25(ctx, cfg, rdb)
		if err != nil {
			return nil, fmt.Errorf("init bm25 index: %w", err)
		}
	}

	var reranker Reranker
	if cfg.Rerank.Enabled {
		reranker = newHTTPReranker(cfg.Rerank)
	}

	log.Printf("[rag] vector_index.provider=%s hybrid=%v rerank=%v",
		cfg.VectorIndex.Provider, cfg.RAG.HybridEnabled, cfg.Rerank.Enabled)

	if cfg.Memory.SummaryEnabled {
		mem.SetSummarizer(&llmSummarizer{chat: chat})
	}

	return &Pipeline{
		cfg:      cfg,
		embedder: emb,
		chat:     chat,
		splitter: splitter,
		store:    store,
		bm25:     bm25,
		reranker: reranker,
		docRepo:  docRepo,
		mem:      mem,
	}, nil
}

// IndexDocument 对已落库文档进行切分、向量化并写入向量索引
func (p *Pipeline) IndexDocument(ctx context.Context, docID uint) error {
	doc, err := p.docRepo.GetByID(docID)
	if err != nil {
		return err
	}

	_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusIndexing, 0, "")

	parsed, err := docparser.ExtractFile(doc.FilePath, doc.ContentType)
	if err != nil {
		_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("parse document: %w", err)
	}
	content := strings.TrimSpace(parsed.Text)
	if content == "" {
		err := fmt.Errorf("parsed document text is empty")
		_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusFailed, 0, err.Error())
		return err
	}

	baseDocs := []*schema.Document{{
		ID:      fmt.Sprintf("doc-%d", doc.ID),
		Content: content,
		MetaData: map[string]any{
			"doc_id":       strconv.FormatUint(uint64(doc.ID), 10),
			"user_id":      doc.UserID,
			"kb_id":        uintToMeta(doc.KnowledgeBaseID),
			"directory_id": ptrUintToMeta(doc.DirectoryID),
			"title":        doc.Title,
			"format":       parsed.Format,
			"content_type": parsed.ContentType,
		},
	}}

	chunks, err := p.splitter.Transform(ctx, baseDocs)
	if err != nil {
		_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("split document: %w", err)
	}

	for i, chunk := range chunks {
		chunk.ID = fmt.Sprintf("%d-%d-%s", doc.ID, i, uuid.NewString()[:8])
		if chunk.MetaData == nil {
			chunk.MetaData = map[string]any{}
		}
		chunk.MetaData["doc_id"] = strconv.FormatUint(uint64(doc.ID), 10)
		chunk.MetaData["user_id"] = doc.UserID
		chunk.MetaData["kb_id"] = uintToMeta(doc.KnowledgeBaseID)
		chunk.MetaData["directory_id"] = ptrUintToMeta(doc.DirectoryID)
		chunk.MetaData["title"] = doc.Title
		chunk.MetaData["chunk_index"] = i
	}

	// 写入前先清掉旧向量，避免 reindex 残留污染检索
	docIDStr := strconv.FormatUint(uint64(doc.ID), 10)
	if err := p.store.DeleteByDocID(ctx, docIDStr); err != nil {
		_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("delete old vectors: %w", err)
	}
	if p.bm25 != nil {
		if err := p.bm25.DeleteByDocID(ctx, docIDStr); err != nil {
			_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusFailed, 0, err.Error())
			return fmt.Errorf("delete old bm25: %w", err)
		}
	}

	if err := p.store.Store(ctx, chunks); err != nil {
		_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusFailed, 0, err.Error())
		return err
	}
	if p.bm25 != nil {
		if err := p.bm25.Upsert(ctx, chunks); err != nil {
			_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusFailed, 0, err.Error())
			return fmt.Errorf("bm25 upsert: %w", err)
		}
	}

	if err := p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusReady, len(chunks), ""); err != nil {
		return err
	}
	log.Printf("[rag] indexed document id=%d chunks=%d provider=%s", docID, len(chunks), p.cfg.VectorIndex.Provider)
	return nil
}

// DeleteDocument 级联删除：向量索引 → 本地文件 → 数据库记录
func (p *Pipeline) DeleteDocument(ctx context.Context, docID uint) error {
	doc, err := p.docRepo.GetByID(docID)
	if err != nil {
		return err
	}
	if doc.Status == dbmodel.DocumentStatusIndexing {
		return fmt.Errorf("文档正在索引中，请稍后再删除")
	}

	docIDStr := strconv.FormatUint(uint64(doc.ID), 10)
	if err := p.store.DeleteByDocID(ctx, docIDStr); err != nil {
		return fmt.Errorf("delete vectors: %w", err)
	}
	if p.bm25 != nil {
		if err := p.bm25.DeleteByDocID(ctx, docIDStr); err != nil {
			return fmt.Errorf("delete bm25: %w", err)
		}
	}

	if doc.FilePath != "" {
		if err := os.Remove(doc.FilePath); err != nil && !os.IsNotExist(err) {
			log.Printf("[rag] remove file failed doc_id=%d path=%s err=%v", docID, doc.FilePath, err)
		}
	}

	if err := p.docRepo.Delete(docID); err != nil {
		return fmt.Errorf("delete document record: %w", err)
	}
	log.Printf("[rag] deleted document id=%d (vectors+file+db)", docID)
	return nil
}

// IndexDocumentAsync 导入完成后异步触发索引构建
func (p *Pipeline) IndexDocumentAsync(docID uint) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := p.IndexDocument(ctx, docID); err != nil {
			log.Printf("[rag] async index failed doc_id=%d err=%v", docID, err)
		}
	}()
}

// QueryRequest RAG 问答请求
type QueryRequest struct {
	UserID          string `json:"user_id"`
	SessionID       string `json:"session_id"`
	Query           string `json:"query"`
	KnowledgeBaseID uint   `json:"knowledge_base_id"`
	DirectoryID     *uint  `json:"directory_id"`
	// Filter 由 service 层组装（含目录子孙展开），handler 可不传
	Filter *RetrieveFilter `json:"-"`
}

// QueryResponse RAG 问答响应
type QueryResponse struct {
	Answer          string           `json:"answer"`
	SessionID       string           `json:"session_id"`
	KnowledgeBaseID uint             `json:"knowledge_base_id,omitempty"`
	DirectoryID     *uint            `json:"directory_id,omitempty"`
	Sources         []SourceDocument `json:"sources"`
}

type SourceDocument struct {
	ID      string  `json:"id"`
	Content string  `json:"content"`
	Title   string  `json:"title,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

// StreamEventType SSE 事件类型
const (
	StreamEventMeta  = "meta"
	StreamEventDelta = "delta"
	StreamEventDone  = "done"
	StreamEventError = "error"
)

// StreamEvent chat/query 流式事件
type StreamEvent struct {
	Type            string           `json:"type"`
	Content         string           `json:"content,omitempty"`
	Answer          string           `json:"answer,omitempty"`
	SessionID       string           `json:"session_id,omitempty"`
	KnowledgeBaseID uint             `json:"knowledge_base_id,omitempty"`
	DirectoryID     *uint            `json:"directory_id,omitempty"`
	Sources         []SourceDocument `json:"sources,omitempty"`
	Message         string           `json:"message,omitempty"`
}

// StreamHandler 流式事件回调；返回 error 时中止生成（如客户端断开）
type StreamHandler func(event StreamEvent) error

// Query 带记忆机制的检索增强生成（聚合完整回答，便于非流式调用）
func (p *Pipeline) Query(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	var resp *QueryResponse
	err := p.QueryStream(ctx, req, func(evt StreamEvent) error {
		switch evt.Type {
		case StreamEventDone:
			resp = &QueryResponse{
				Answer:          evt.Answer,
				SessionID:       evt.SessionID,
				KnowledgeBaseID: evt.KnowledgeBaseID,
				DirectoryID:     evt.DirectoryID,
				Sources:         evt.Sources,
			}
		case StreamEventError:
			return fmt.Errorf("%s", evt.Message)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("empty stream response")
	}
	return resp, nil
}

// QueryStream 流式问答：先推送 meta（session/sources），再推送 delta，最后 done
func (p *Pipeline) QueryStream(ctx context.Context, req QueryRequest, onEvent StreamHandler) error {
	if onEvent == nil {
		return fmt.Errorf("stream handler is required")
	}
	if req.SessionID == "" {
		req.SessionID = uuid.NewString()
	}
	if req.UserID == "" {
		req.UserID = "anonymous"
	}

	docs, err := p.retrieve(ctx, req.Query, req.Filter)
	if err != nil {
		return err
	}

	history, err := p.mem.BuildContextForPrompt(ctx, req.SessionID)
	if err != nil {
		return fmt.Errorf("load memory: %w", err)
	}

	sources := make([]SourceDocument, 0, len(docs))
	for _, d := range docs {
		sources = append(sources, SourceDocument{
			ID:      d.ID,
			Content: truncate(d.Content, 300),
			Title:   metaString(d.MetaData, "title"),
			Score:   d.Score(),
		})
	}

	if err := onEvent(StreamEvent{
		Type:            StreamEventMeta,
		SessionID:       req.SessionID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		DirectoryID:     req.DirectoryID,
		Sources:         sources,
	}); err != nil {
		return err
	}

	answer, err := p.generateStream(ctx, req.Query, docs, history, func(delta string) error {
		return onEvent(StreamEvent{Type: StreamEventDelta, Content: delta})
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) == "" {
		answer = emptyAnswerFallback(docs)
		if err := onEvent(StreamEvent{Type: StreamEventDelta, Content: answer}); err != nil {
			return err
		}
	}

	if err := p.mem.Append(ctx, req.UserID, req.SessionID, dbmodel.RoleUser, req.Query); err != nil {
		log.Printf("[rag] append user memory failed: %v", err)
	}
	if err := p.mem.Append(ctx, req.UserID, req.SessionID, dbmodel.RoleAssistant, answer); err != nil {
		log.Printf("[rag] append assistant memory failed: %v", err)
	}

	return onEvent(StreamEvent{
		Type:            StreamEventDone,
		Answer:          answer,
		SessionID:       req.SessionID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		DirectoryID:     req.DirectoryID,
		Sources:         sources,
	})
}

// retrieve 稠密召回 →（可选）BM25 + RRF →（可选）Rerank → TopK
func (p *Pipeline) retrieve(ctx context.Context, query string, filter *RetrieveFilter) ([]*schema.Document, error) {
	topK := p.cfg.RAG.TopK
	if topK <= 0 {
		topK = 5
	}
	needPool := p.cfg.RAG.HybridEnabled || p.cfg.Rerank.Enabled
	poolK := topK
	if needPool {
		poolK = candidateK(topK, p.cfg.RAG.CandidateK)
	}

	dense, err := p.store.Retrieve(ctx, query, filter, poolK)
	if err != nil {
		return nil, err
	}

	fused := dense
	if p.cfg.RAG.HybridEnabled && p.bm25 != nil {
		sparse, serr := p.bm25.Search(ctx, query, filter, poolK)
		if serr != nil {
			log.Printf("[rag] bm25 search failed, fallback dense-only: %v", serr)
		} else if len(sparse) > 0 {
			fused = fuseRRF([][]*schema.Document{dense, sparse}, p.cfg.RAG.RRFK)
			log.Printf("[rag] hybrid fuse dense=%d bm25=%d fused=%d", len(dense), len(sparse), len(fused))
		}
	}

	if p.cfg.Rerank.Enabled && p.reranker != nil && len(fused) > 0 {
		rerankTopN := p.cfg.Rerank.TopN
		if rerankTopN <= 0 {
			rerankTopN = topK
		}
		// 重排输入截断到候选池，避免过长请求
		candidates := truncateDocs(fused, poolK)
		reranked, rerr := p.reranker.Rerank(ctx, query, candidates, rerankTopN)
		if rerr != nil {
			log.Printf("[rag] rerank failed, fallback fused top_k: %v", rerr)
			return truncateDocs(fused, topK), nil
		}
		log.Printf("[rag] reranked candidates=%d -> %d", len(candidates), len(reranked))
		return reranked, nil
	}

	return truncateDocs(fused, topK), nil
}

func (p *Pipeline) buildMessages(
	query string,
	docs []*schema.Document,
	history *memory.ContextPack,
) []*schema.Message {
	var ctxBuilder strings.Builder
	if len(docs) == 0 {
		ctxBuilder.WriteString("（未检索到相关知识库片段）")
	} else {
		for i, d := range docs {
			fmt.Fprintf(&ctxBuilder, "[%d] 标题:%s\n%s\n\n", i+1, metaString(d.MetaData, "title"), d.Content)
		}
	}

	messages := []*schema.Message{
		{
			Role: schema.System,
			Content: `你是企业知识库助手。请仅依据提供的「知识库上下文」与「历史对话」回答用户问题。
若上下文不足以回答，请明确说明「根据现有知识库无法确定」，不要编造。
回答使用简洁中文，必要时引用片段编号如 [1]。`,
		},
	}

	if history != nil && history.Summary != "" {
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: "历史对话摘要:\n" + history.Summary,
		})
	}

	var turns []memory.ChatTurn
	if history != nil {
		turns = history.Turns
	}
	for _, turn := range turns {
		role := schema.User
		switch turn.Role {
		case dbmodel.RoleAssistant:
			role = schema.Assistant
		case dbmodel.RoleSystem:
			role = schema.System
		}
		messages = append(messages, &schema.Message{Role: role, Content: turn.Content})
	}

	messages = append(messages, &schema.Message{
		Role:    schema.User,
		Content: fmt.Sprintf("知识库上下文:\n%s\n\n用户问题: %s", ctxBuilder.String(), query),
	})
	return messages
}

func (p *Pipeline) generate(
	ctx context.Context,
	query string,
	docs []*schema.Document,
	history *memory.ContextPack,
) (string, error) {
	resp, err := p.chat.Generate(ctx, p.buildMessages(query, docs, history))
	if err != nil {
		return "", fmt.Errorf("deepseek generate: %w", err)
	}
	return resp.Content, nil
}

func (p *Pipeline) generateStream(
	ctx context.Context,
	query string,
	docs []*schema.Document,
	history *memory.ContextPack,
	onDelta func(string) error,
) (string, error) {
	reader, err := p.chat.Stream(ctx, p.buildMessages(query, docs, history))
	if err != nil {
		return "", fmt.Errorf("deepseek stream: %w", err)
	}
	defer reader.Close()

	var full strings.Builder
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return full.String(), fmt.Errorf("deepseek stream recv: %w", err)
		}
		if chunk == nil || chunk.Content == "" {
			continue
		}
		full.WriteString(chunk.Content)
		if onDelta != nil {
			if err := onDelta(chunk.Content); err != nil {
				return full.String(), err
			}
		}
	}
	return full.String(), nil
}

// SaveUpload 将上传文件持久化到本地
func (p *Pipeline) SaveUpload(userID, title, fileName string, data []byte) (string, error) {
	dir := filepath.Join(p.cfg.RAG.UploadDir, userID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	safeName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(fileName))
	path := filepath.Join(dir, safeName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	_ = title
	return path, nil
}

func metaString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

func emptyAnswerFallback(docs []*schema.Document) string {
	if len(docs) == 0 {
		return "根据现有知识库无法确定。"
	}
	garbled := 0
	for _, d := range docs {
		if looksLikeGarbledChunk(d.Content) {
			garbled++
		}
	}
	if garbled*2 >= len(docs) {
		return "检索到的知识库片段无法阅读（多为 PDF 字体未正确解码）。请对该 PDF 执行「重新索引」后再提问。"
	}
	return "根据现有知识库无法确定。"
}

func looksLikeGarbledChunk(s string) bool {
	if s == "" {
		return true
	}
	ctrl := 0
	han := 0
	total := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if r < 0x20 {
			ctrl++
		}
		if unicode.Is(unicode.Han, r) {
			han++
		}
	}
	if total == 0 {
		return true
	}
	if ctrl > 5 {
		return true
	}
	// 来源标题常见中文 PDF，正文几乎无汉字
	return han == 0 && total > 40
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
