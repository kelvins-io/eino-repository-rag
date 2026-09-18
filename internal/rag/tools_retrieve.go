package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

const knowledgeRetrieveToolName = "knowledge_retrieve"

type ctxKeyFilter struct{}
type ctxKeyCollector struct{}

// agentDocCollector 跨多轮 tool 调用累积召回文档，并分配全局引用编号。
// rounds 保留每一轮 knowledge_retrieve 的原始顺序，供文档召回率使用，不使用合并后的编号当排名。
type agentDocCollector struct {
	mu     sync.Mutex
	docs   []*schema.Document
	byID   map[string]int // doc.ID -> 1-based index
	rounds [][]*schema.Document
}

func newAgentDocCollector() *agentDocCollector {
	return &agentDocCollector{byID: make(map[string]int)}
}

func (c *agentDocCollector) Merge(docs []*schema.Document) (ordered []*schema.Document, indices []int) {
	if c == nil {
		return nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range docs {
		if d == nil {
			continue
		}
		id := chunkKey(d)
		if id == "" {
			continue
		}
		if idx, ok := c.byID[id]; ok {
			indices = append(indices, idx)
			ordered = append(ordered, c.docs[idx-1])
			continue
		}
		c.docs = append(c.docs, d)
		idx := len(c.docs)
		c.byID[id] = idx
		indices = append(indices, idx)
		ordered = append(ordered, d)
	}
	return ordered, indices
}

func (c *agentDocCollector) Docs() []*schema.Document {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*schema.Document, len(c.docs))
	copy(out, c.docs)
	return out
}

// RecordRound 记下这一轮检索的原始顺序。必须在 Merge 之前调用。
func (c *agentDocCollector) RecordRound(docs []*schema.Document) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]*schema.Document, len(docs))
	copy(cp, docs)
	c.rounds = append(c.rounds, cp)
}

func (c *agentDocCollector) Rounds() [][]*schema.Document {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]*schema.Document, len(c.rounds))
	for i, round := range c.rounds {
		cp := make([]*schema.Document, len(round))
		copy(cp, round)
		out[i] = cp
	}
	return out
}

func (c *agentDocCollector) GlobalIndex(d *schema.Document) int {
	if c == nil || d == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byID[chunkKey(d)]
}

func chunkKey(d *schema.Document) string {
	if d == nil {
		return ""
	}
	if d.ID != "" {
		return d.ID
	}
	id := metaString(d.MetaData, "doc_id") + ":" + metaString(d.MetaData, "chunk_index")
	if id == ":" {
		return ""
	}
	return id
}

func withAgentRetrieveContext(ctx context.Context, filter *RetrieveFilter, collector *agentDocCollector) context.Context {
	ctx = context.WithValue(ctx, ctxKeyFilter{}, filter)
	ctx = context.WithValue(ctx, ctxKeyCollector{}, collector)
	return ctx
}

func filterFromAgentCtx(ctx context.Context) *RetrieveFilter {
	v, _ := ctx.Value(ctxKeyFilter{}).(*RetrieveFilter)
	return v
}

func collectorFromAgentCtx(ctx context.Context) *agentDocCollector {
	v, _ := ctx.Value(ctxKeyCollector{}).(*agentDocCollector)
	return v
}

// knowledgeRetrieveInput tool 入参；禁止传 tenant/kb，强制用请求上下文 Filter。
type knowledgeRetrieveInput struct {
	Query string `json:"query" jsonschema:"required,description=用于知识库检索的查询语句，可改写或拆分关键词"`
}

type knowledgeRetrieveOutput struct {
	Count   int    `json:"count"`
	Summary string `json:"summary"`
}

func (p *Pipeline) newKnowledgeRetrieveTool(topK int) (tool.BaseTool, error) {
	if topK <= 0 {
		topK = p.cfg.RAG.TopK
		if topK <= 0 {
			topK = 5
		}
	}
	return utils.InferTool(
		knowledgeRetrieveToolName,
		"在企业知识库中检索与问题相关的文档片段。需要事实依据时必须调用本工具；可多次调用以补充不同角度。",
		func(ctx context.Context, in knowledgeRetrieveInput) (knowledgeRetrieveOutput, error) {
			return p.runKnowledgeRetrieve(ctx, strings.TrimSpace(in.Query), topK)
		},
	)
}

func (p *Pipeline) runKnowledgeRetrieve(ctx context.Context, query string, topK int) (knowledgeRetrieveOutput, error) {
	if query == "" {
		return knowledgeRetrieveOutput{Summary: "query 为空，未检索"}, nil
	}
	filter := filterFromAgentCtx(ctx)
	if filter == nil || filter.TenantID == "" {
		return knowledgeRetrieveOutput{}, fmt.Errorf("knowledge_retrieve: missing tenant filter in context")
	}
	docs, err := p.Retrieve(ctx, query, filter)
	if err != nil {
		return knowledgeRetrieveOutput{}, err
	}
	docs = truncateDocs(docs, topK)
	collector := collectorFromAgentCtx(ctx)
	collector.RecordRound(docs)
	ordered, indices := collector.Merge(docs)
	summary := formatRetrieveSummary(ordered, indices)
	return knowledgeRetrieveOutput{
		Count:   len(ordered),
		Summary: summary,
	}, nil
}

func formatRetrieveSummary(docs []*schema.Document, indices []int) string {
	if len(docs) == 0 {
		return "（未检索到相关片段）"
	}
	var b strings.Builder
	for i, d := range docs {
		n := i + 1
		if i < len(indices) {
			n = indices[i]
		}
		page := metaInt(d.MetaData, "page")
		sec := metaString(d.MetaData, "section")
		loc := fmt.Sprintf("doc_id=%s", metaString(d.MetaData, "doc_id"))
		if page > 0 {
			loc += fmt.Sprintf(" page=%d", page)
		}
		if sec != "" {
			loc += fmt.Sprintf(" section=%s", sec)
		}
		fmt.Fprintf(&b, "[%d] 标题:%s (%s)\n%s\n\n",
			n, metaString(d.MetaData, "title"), loc, truncate(d.Content, 600))
	}
	return strings.TrimSpace(b.String())
}

// parseToolQueryFromArgs 从 tool 回调 JSON 参数中提取 query（供 SSE 展示）。
func parseToolQueryFromArgs(argumentsJSON string) string {
	argumentsJSON = strings.TrimSpace(argumentsJSON)
	if argumentsJSON == "" {
		return ""
	}
	var in knowledgeRetrieveInput
	if err := json.Unmarshal([]byte(argumentsJSON), &in); err == nil {
		return strings.TrimSpace(in.Query)
	}
	return ""
}

// mergeDocsByID 按 ID 去重合并多路文档（保留先出现者）。
func mergeDocsByID(lists ...[]*schema.Document) []*schema.Document {
	seen := map[string]struct{}{}
	var out []*schema.Document
	for _, list := range lists {
		for _, d := range list {
			if d == nil {
				continue
			}
			id := d.ID
			if id == "" {
				id = metaString(d.MetaData, "doc_id") + ":" + metaString(d.MetaData, "chunk_index")
			}
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, d)
		}
	}
	return out
}
