package rag

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	"github.com/kelvins-io/eino-repository-rag/internal/memory"
	dbmodel "github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/rag/parser"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

// Pipeline 基于 Eino 的企业知识库 RAG 流水线
type Pipeline struct {
	cfg        *config.Config
	embedder   embedding.Embedder
	chat       einomodel.BaseChatModel
	toolChat   einomodel.ToolCallingChatModel // Agent 用；DeepSeek 实现此接口
	splitter   document.Transformer
	store      VectorStore
	bm25       *redisBM25
	reranker   Reranker
	expander   QueryExpander
	docRepo    *repository.DocumentRepo
	mem        *memory.Manager
	indexQueue *IndexQueue
}

func NewPipeline(
	ctx context.Context,
	cfg *config.Config,
	rdb *redis.Client,
	docRepo *repository.DocumentRepo,
	mem *memory.Manager,
) (*Pipeline, error) {
	dim := cfg.Embedding.Dimensions
	embTimeout := time.Duration(cfg.Embedding.TimeoutSeconds) * time.Second
	if embTimeout <= 0 {
		embTimeout = 120 * time.Second
	}
	embCfg := &openai.EmbeddingConfig{
		APIKey:  cfg.Embedding.APIKey,
		Model:   cfg.Embedding.Model,
		Timeout: embTimeout,
	}
	if cfg.Embedding.BaseURL != "" {
		embCfg.BaseURL = cfg.Embedding.BaseURL
	}
	if dim > 0 {
		embCfg.Dimensions = &dim
	}
	rawEmb, err := openai.NewEmbedder(ctx, embCfg)
	if err != nil {
		return nil, fmt.Errorf("create embedder: %w", err)
	}
	emb := newResilientEmbedder(rawEmb, cfg.Embedding)

	chatTimeout := time.Duration(cfg.DeepSeek.TimeoutSeconds) * time.Second
	if chatTimeout <= 0 {
		chatTimeout = 120 * time.Second
	}
	chat, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:      cfg.DeepSeek.APIKey,
		Model:       cfg.DeepSeek.Model,
		BaseURL:     cfg.DeepSeek.BaseURL,
		MaxTokens:   cfg.DeepSeek.MaxTokens,
		Temperature: cfg.DeepSeek.Temperature,
		Timeout:     chatTimeout,
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

	var expander QueryExpander
	if cfg.RAG.QueryExpandEnabled {
		expander = newLLMQueryExpander(chat, cfg.RAG.QueryExpandTimeoutSeconds)
	}

	logger.S().Infof("[rag] vector_index.provider=%s hybrid=%v rerank=%v query_expand=%v(n=%d) structure_split=%v citation_validate=%v agent=%v ocr=%v endpoint=%s chat_timeout=%s embed_timeout=%s rerank_timeout=%ds ocr_timeout=%ds expand_timeout=%ds summary_timeout=%ds embed_batch=%d embed_retries=%d embed_concurrency=%d",
		cfg.VectorIndex.Provider, cfg.RAG.HybridEnabled, cfg.Rerank.Enabled,
		cfg.RAG.QueryExpandEnabled, cfg.RAG.QueryExpandN,
		cfg.RAG.StructureSplitEnabled, cfg.RAG.CitationValidateEnabled, cfg.Agent.Enabled,
		cfg.RAG.OCR.Enabled, cfg.RAG.OCR.Endpoint,
		chatTimeout, embTimeout, cfg.Rerank.TimeoutSeconds, cfg.RAG.OCR.TimeoutSeconds,
		cfg.RAG.QueryExpandTimeoutSeconds, cfg.Memory.SummaryTimeoutSeconds,
		cfg.Embedding.BatchSize, cfg.Embedding.MaxRetries, cfg.Embedding.MaxConcurrency)

	if cfg.Memory.SummaryEnabled {
		sumTimeout := time.Duration(cfg.Memory.SummaryTimeoutSeconds) * time.Second
		if sumTimeout <= 0 {
			sumTimeout = 30 * time.Second
		}
		mem.SetSummarizer(&llmSummarizer{chat: chat, timeout: sumTimeout})
	}

	p := &Pipeline{
		cfg:      cfg,
		embedder: emb,
		chat:     chat,
		toolChat: chat, // deepseek.ChatModel 实现 ToolCallingChatModel
		splitter: splitter,
		store:    store,
		bm25:     bm25,
		reranker: reranker,
		expander: expander,
		docRepo:  docRepo,
		mem:      mem,
	}
	p.indexQueue = NewIndexQueue(cfg, rdb, p, docRepo)
	return p, nil
}

// IndexDocument 对已落库文档进行切分、向量化并写入向量索引
func (p *Pipeline) IndexDocument(ctx context.Context, docID uint) error {
	doc, err := p.docRepo.GetByID(docID)
	if err != nil {
		return err
	}

	if err := p.docRepo.BeginIndexBuild(doc); err != nil {
		logger.S().Errorf("[rag] begin index build doc_id=%d err=%v", docID, err)
	}
	_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusIndexing, 0, "")

	parsed, err := parser.ExtractFile(doc.FilePath, doc.ContentType,
		parser.WithContext(ctx),
		parser.WithOCR(ocrFromConfig(p.cfg.RAG.OCR)),
	)
	if err != nil {
		p.failDocument(ctx, docID, err.Error())
		return fmt.Errorf("parse file: %w", err)
	}
	if strings.TrimSpace(parsed.Text) == "" {
		errMsg := "parsed text is empty"
		p.failDocument(ctx, docID, errMsg)
		return fmt.Errorf("parse file: %s", errMsg)
	}

	chunks, err := splitDocument(ctx, p.splitter, parsed.Text, parsed.Format, p.cfg.RAG.ChunkSize, p.cfg.RAG.StructureSplitEnabled)
	if err != nil {
		p.failDocument(ctx, docID, err.Error())
		return fmt.Errorf("split document: %w", err)
	}
	if len(chunks) == 0 {
		errMsg := "split produced no chunks"
		p.failDocument(ctx, docID, errMsg)
		return fmt.Errorf("split document: %s", errMsg)
	}

	for i, chunk := range chunks {
		chunk.ID = fmt.Sprintf("%d-%d-%s", doc.ID, i, uuid.NewString()[:8])
		if chunk.MetaData == nil {
			chunk.MetaData = map[string]any{}
		}
		chunk.MetaData["doc_id"] = strconv.FormatUint(uint64(doc.ID), 10)
		chunk.MetaData["tenant_id"] = uintToMeta(doc.TenantID)
		chunk.MetaData["user_id"] = doc.UserID
		chunk.MetaData["kb_id"] = uintToMeta(doc.KnowledgeBaseID)
		chunk.MetaData["directory_id"] = ptrUintToMeta(doc.DirectoryID)
		chunk.MetaData["title"] = doc.Title
		chunk.MetaData["format"] = parsed.Format
		chunk.MetaData["chunk_index"] = i
		if page := metaInt(chunk.MetaData, "page"); page <= 0 {
			if page = inferPageFromContent(chunk.Content); page > 0 {
				chunk.MetaData["page"] = page
			}
		}
	}

	// 写入前先清掉旧向量，避免 reindex 残留污染检索
	docIDStr := strconv.FormatUint(uint64(doc.ID), 10)
	if err := p.store.DeleteByDocID(ctx, docIDStr); err != nil {
		p.failDocument(ctx, docID, err.Error())
		return fmt.Errorf("delete old vectors: %w", err)
	}
	if p.bm25 != nil {
		if err := p.bm25.DeleteByDocID(ctx, docIDStr); err != nil {
			p.failDocument(ctx, docID, err.Error())
			return fmt.Errorf("delete old bm25: %w", err)
		}
	}

	logger.S().Infof("[rag] indexing document id=%d format=%s chunks=%d", docID, parsed.Format, len(chunks))
	if err := p.store.Store(ctx, chunks); err != nil {
		p.failDocument(ctx, docID, err.Error())
		return err
	}
	if p.bm25 != nil {
		if err := p.bm25.Upsert(ctx, chunks); err != nil {
			p.failDocument(ctx, docID, err.Error())
			return fmt.Errorf("bm25 upsert: %w", err)
		}
	}

	if err := p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusReady, len(chunks), ""); err != nil {
		return err
	}
	logger.S().Infof("[rag] indexed document id=%d format=%s chunks=%d provider=%s",
		docID, parsed.Format, len(chunks), p.cfg.VectorIndex.Provider)
	return nil
}

// failDocument 将文档标为失败。关停取消（Canceled）时跳过，避免把可回灌的在途任务写成 Failed。
func (p *Pipeline) failDocument(ctx context.Context, docID uint, errMsg string) {
	if errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	_ = p.docRepo.UpdateStatus(docID, dbmodel.DocumentStatusFailed, 0, errMsg)
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

	if path := strings.TrimSpace(doc.FilePath); path != "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete local file %s: %w", path, err)
		}
	}

	if err := p.docRepo.Delete(docID); err != nil {
		return fmt.Errorf("delete document record: %w", err)
	}
	logger.S().Infof("[rag] deleted document id=%d (vectors+file+db)", docID)
	return nil
}

// IndexDocumentAsync 将文档加入 Redis 索引队列（异步构建）
func (p *Pipeline) IndexDocumentAsync(docID uint) {
	if p.indexQueue == nil {
		logger.S().Warnf("[rag] index queue not ready, skip doc_id=%d", docID)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.indexQueue.Enqueue(ctx, docID); err != nil {
		logger.S().Errorf("[rag] index enqueue failed doc_id=%d err=%v", docID, err)
	}
}

// StartIndexQueue 启动索引队列 worker（含崩溃回灌）
func (p *Pipeline) StartIndexQueue(ctx context.Context) error {
	if p.indexQueue == nil {
		return nil
	}
	return p.indexQueue.Start(ctx)
}

// StopIndexQueue 停止索引队列 worker
func (p *Pipeline) StopIndexQueue() {
	if p.indexQueue != nil {
		p.indexQueue.Stop()
	}
}

// QueryRequest RAG 问答请求
type QueryRequest struct {
	UserID          string `json:"user_id"`
	TenantID        uint   `json:"-"` // 由 handler 从 JWT 注入
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
	ID         string  `json:"id"`
	DocID      string  `json:"doc_id,omitempty"`
	ChunkIndex int     `json:"chunk_index"`
	Page       int     `json:"page,omitempty"` // PDF/PPTX 页码；0 表示未知
	Title      string  `json:"title,omitempty"`
	Section    string  `json:"section,omitempty"` // 结构切分得到的章节/页/工作表名
	Format     string  `json:"format,omitempty"`
	Content    string  `json:"content"`
	Score      float64 `json:"score,omitempty"`
}

// StreamEventType SSE 事件类型
const (
	StreamEventMeta       = "meta"
	StreamEventDelta      = "delta"
	StreamEventDone       = "done"
	StreamEventError      = "error"
	StreamEventStep       = "step"
	StreamEventToolStart  = "tool_start"
	StreamEventToolResult = "tool_result"
)

// StreamEvent chat/query / chat/agent 流式事件
type StreamEvent struct {
	Type            string           `json:"type"`
	Content         string           `json:"content,omitempty"`
	Answer          string           `json:"answer,omitempty"`
	SessionID       string           `json:"session_id,omitempty"`
	KnowledgeBaseID uint             `json:"knowledge_base_id,omitempty"`
	DirectoryID     *uint            `json:"directory_id,omitempty"`
	Sources         []SourceDocument `json:"sources,omitempty"`
	Message         string           `json:"message,omitempty"`
	Step            int              `json:"step,omitempty"`
	Tool            string           `json:"tool,omitempty"`
	ToolQuery       string           `json:"tool_query,omitempty"`
	ToolCount       int              `json:"tool_count,omitempty"`
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
	// 强制租户过滤；共享知识库检索按 tenant_id，不按上传者 user_id 收窄
	tenantMeta := uintToMeta(req.TenantID)
	if req.Filter == nil {
		req.Filter = &RetrieveFilter{TenantID: tenantMeta}
	} else if req.Filter.TenantID == "" {
		req.Filter.TenantID = tenantMeta
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
		sources = append(sources, buildSourceDocument(d))
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

	finalSources := sources
	if p.cfg.RAG.CitationValidateEnabled {
		check := validateCitations(answer, len(sources))
		if check.Changed {
			logger.S().Infof("[rag] citation validate removed=%v kept=%v", check.Removed, check.ValidCited)
			answer = check.Answer
			if p.cfg.RAG.CitationFilterSources {
				finalSources = filterSourcesByCited(sources, check.ValidCited)
			}
		}
	}

	if err := p.mem.Append(ctx, req.TenantID, req.UserID, req.SessionID, dbmodel.RoleUser, req.Query, req.KnowledgeBaseID, req.DirectoryID); err != nil {
		logger.S().Errorf("[rag] append user memory failed: %v", err)
	}
	if err := p.mem.Append(ctx, req.TenantID, req.UserID, req.SessionID, dbmodel.RoleAssistant, answer, req.KnowledgeBaseID, req.DirectoryID); err != nil {
		logger.S().Errorf("[rag] append assistant memory failed: %v", err)
	}

	return onEvent(StreamEvent{
		Type:            StreamEventDone,
		Answer:          answer,
		SessionID:       req.SessionID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		DirectoryID:     req.DirectoryID,
		Sources:         finalSources,
	})
}

// Retrieve 仅检索不生成（供评测 / 调试）；走完整 expand → hybrid → rerank 路径。
func (p *Pipeline) Retrieve(ctx context.Context, query string, filter *RetrieveFilter) ([]*schema.Document, error) {
	return p.retrieve(ctx, query, filter)
}

// retrieve 可选 Query 改写多路召回 → 每路 Dense(+BM25/RRF) → 跨路 RRF →（可选）Rerank → TopK
func (p *Pipeline) retrieve(ctx context.Context, query string, filter *RetrieveFilter) ([]*schema.Document, error) {
	topK := p.cfg.RAG.TopK
	if topK <= 0 {
		topK = 5
	}
	needPool := p.cfg.RAG.HybridEnabled || p.cfg.Rerank.Enabled || p.cfg.RAG.QueryExpandEnabled
	poolK := topK
	if needPool {
		poolK = candidateK(topK, p.cfg.RAG.CandidateK)
	}

	queries := []string{strings.TrimSpace(query)}
	if p.cfg.RAG.QueryExpandEnabled && p.expander != nil {
		n := p.cfg.RAG.QueryExpandN
		if n <= 0 {
			n = 2
		}
		variants, err := p.expander.Expand(ctx, query, n)
		if err != nil {
			logger.S().Warnf("[rag] query expand failed, fallback single query: %v", err)
		} else if len(variants) > 0 {
			queries = buildExpandQueries(query, variants)
			logger.S().Infof("[rag] query expand original=%q variants=%v", query, variants)
		}
	}

	lists := make([][]*schema.Document, 0, len(queries))
	for _, q := range queries {
		docs, err := p.retrieveOne(ctx, q, filter, poolK)
		if err != nil {
			return nil, err
		}
		if len(docs) > 0 {
			lists = append(lists, docs)
		}
	}
	if len(lists) == 0 {
		return nil, nil
	}

	fused := lists[0]
	if len(lists) > 1 {
		fused = fuseRRF(lists, p.cfg.RAG.RRFK)
		logger.S().Infof("[rag] multi-query fuse paths=%d fused=%d", len(lists), len(fused))
	}

	if p.cfg.Rerank.Enabled && p.reranker != nil && len(fused) > 0 {
		rerankTopN := p.cfg.Rerank.TopN
		if rerankTopN <= 0 {
			rerankTopN = topK
		}
		// 重排始终用原始用户问题，避免改写偏移意图
		candidates := truncateDocs(fused, poolK)
		reranked, rerr := p.reranker.Rerank(ctx, query, candidates, rerankTopN)
		if rerr != nil {
			logger.S().Warnf("[rag] rerank failed, fallback fused top_k: %v", rerr)
			return truncateDocs(fused, topK), nil
		}
		logger.S().Infof("[rag] reranked candidates=%d -> %d", len(candidates), len(reranked))
		return reranked, nil
	}

	return truncateDocs(fused, topK), nil
}

// retrieveOne 单路：稠密召回 →（可选）BM25 + RRF
func (p *Pipeline) retrieveOne(ctx context.Context, query string, filter *RetrieveFilter, poolK int) ([]*schema.Document, error) {
	dense, err := p.store.Retrieve(ctx, query, filter, poolK)
	if err != nil {
		return nil, err
	}

	fused := dense
	if p.cfg.RAG.HybridEnabled && p.bm25 != nil {
		sparse, serr := p.bm25.Search(ctx, query, filter, poolK)
		if serr != nil {
			logger.S().Warnf("[rag] bm25 search failed, fallback dense-only: %v", serr)
		} else if len(sparse) > 0 {
			fused = fuseRRF([][]*schema.Document{dense, sparse}, p.cfg.RAG.RRFK)
			logger.S().Infof("[rag] hybrid fuse dense=%d bm25=%d fused=%d", len(dense), len(sparse), len(fused))
		}
	}
	return fused, nil
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
			page := metaInt(d.MetaData, "page")
			if page <= 0 {
				page = inferPageFromContent(d.Content)
			}
			loc := fmt.Sprintf("doc_id=%s", metaString(d.MetaData, "doc_id"))
			if page > 0 {
				loc += fmt.Sprintf(" page=%d", page)
			}
			if sec := metaString(d.MetaData, "section"); sec != "" {
				loc += fmt.Sprintf(" section=%s", sec)
			}
			fmt.Fprintf(&ctxBuilder, "[%d] 标题:%s (%s)\n%s\n\n",
				i+1, metaString(d.MetaData, "title"), loc, d.Content)
		}
	}

	messages := []*schema.Message{
		{
			Role: schema.System,
			Content: `你是企业知识库助手。请仅依据提供的「知识库上下文」与「历史对话」回答用户问题。
若上下文不足以回答，请明确说明「根据现有知识库无法确定」，不要编造。
回答使用简洁中文。引用时只能使用上下文中已有的片段编号，格式为 [1]、[2]……；禁止引用不存在的编号，禁止编造来源。`,
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

// SaveUpload 将上传文件持久化到本地：{upload_dir}/{TenantID}/{KnowledgeBaseID}/{纳秒时间戳}_{原始文件名}
func (p *Pipeline) SaveUpload(tenantID, knowledgeBaseID uint, fileName string, data []byte) (string, error) {
	dir := filepath.Join(p.cfg.RAG.UploadDir, fmt.Sprintf("%d", tenantID), fmt.Sprintf("%d", knowledgeBaseID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	safeName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(fileName))
	path := filepath.Join(dir, safeName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
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

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func ocrFromConfig(cfg config.OCRConfig) parser.OCR {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return parser.OCR{
		Enabled:      cfg.Enabled,
		Endpoint:     cfg.Endpoint,
		Languages:    cfg.Languages,
		DPI:          cfg.DPI,
		Concurrency:  cfg.Concurrency,
		Timeout:      timeout,
		PageSegMode:  cfg.PageSegMode,
		TesseractBin: cfg.TesseractBin,
		PDFToPPMBin:  cfg.PDFToPPMBin,
	}
}
