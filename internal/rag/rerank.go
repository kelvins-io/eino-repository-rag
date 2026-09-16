package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

// Reranker 对候选文档做交叉编码重排
type Reranker interface {
	Rerank(ctx context.Context, query string, docs []*schema.Document, topN int) ([]*schema.Document, error)
}

type httpReranker struct {
	cfg    config.RerankConfig
	client *http.Client
}

func newHTTPReranker(cfg config.RerankConfig) *httpReranker {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &httpReranker{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

type rerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n,omitempty"`
	ReturnDocuments bool     `json:"return_documents"`
}

type rerankResponse struct {
	Results []rerankResult `json:"results"`
	Data    []rerankResult `json:"data"` // 部分兼容实现
}

type rerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
	Score          float64 `json:"score"`
}

func (r *httpReranker) Rerank(ctx context.Context, query string, docs []*schema.Document, topN int) ([]*schema.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	if topN <= 0 || topN > len(docs) {
		topN = len(docs)
	}
	if strings.TrimSpace(r.cfg.APIKey) == "" {
		return nil, fmt.Errorf("rerank api_key is empty")
	}
	base := strings.TrimRight(strings.TrimSpace(r.cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("rerank base_url is empty")
	}
	url := base + "/rerank"

	texts := make([]string, len(docs))
	for i, d := range docs {
		title := metaString(d.MetaData, "title")
		if title != "" {
			texts[i] = title + "\n" + d.Content
		} else {
			texts[i] = d.Content
		}
	}

	body, err := json.Marshal(rerankRequest{
		Model:           r.cfg.Model,
		Query:           query,
		Documents:       texts,
		TopN:            topN,
		ReturnDocuments: false,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.cfg.APIKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rerank response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rerank http %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	var parsed rerankResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w", err)
	}
	results := parsed.Results
	if len(results) == 0 {
		results = parsed.Data
	}
	if len(results) == 0 {
		return truncateDocs(docs, topN), nil
	}

	out := make([]*schema.Document, 0, len(results))
	for _, item := range results {
		if item.Index < 0 || item.Index >= len(docs) {
			continue
		}
		doc := cloneDoc(docs[item.Index])
		score := item.RelevanceScore
		if score == 0 {
			score = item.Score
		}
		doc.WithScore(score)
		if doc.MetaData == nil {
			doc.MetaData = map[string]any{}
		}
		doc.MetaData["channel"] = "rerank"
		out = append(out, doc)
		if len(out) >= topN {
			break
		}
	}
	if len(out) == 0 {
		return truncateDocs(docs, topN), nil
	}
	return out, nil
}
