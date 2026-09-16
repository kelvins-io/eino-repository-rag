package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// QueryExpander 将用户问题扩展为多条检索查询（改写 / 多路召回）。
type QueryExpander interface {
	Expand(ctx context.Context, query string, n int) ([]string, error)
}

// llmQueryExpander 使用聊天模型生成释义/同义检索问法。
type llmQueryExpander struct {
	chat    einomodel.BaseChatModel
	timeout time.Duration
}

func newLLMQueryExpander(chat einomodel.BaseChatModel, timeoutSec int) *llmQueryExpander {
	d := time.Duration(timeoutSec) * time.Second
	if d <= 0 {
		d = 20 * time.Second
	}
	return &llmQueryExpander{chat: chat, timeout: d}
}

func (e *llmQueryExpander) Expand(ctx context.Context, query string, n int) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" || n <= 0 {
		return nil, nil
	}
	if e.chat == nil {
		return nil, fmt.Errorf("query expander: chat model is nil")
	}

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	messages := []*schema.Message{
		{
			Role: schema.System,
			Content: `你是检索查询改写助手。根据用户原问题，生成若干条用于向量/关键词检索的改写问法。
要求：
1. 保持原意，可换同义词、拆成关键词短语、补全隐含实体。
2. 每条简短，适合检索，不要回答问题。
3. 不要与原问题完全相同；条数恰好为用户指定的 N。
4. 只输出 JSON 字符串数组，例如 ["改写1","改写2"]，不要 markdown、不要解释。`,
		},
		{
			Role:    schema.User,
			Content: fmt.Sprintf("N=%d\n原问题：%s", n, query),
		},
	}

	resp, err := e.chat.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("query expand generate: %w", err)
	}
	variants := parseExpandJSON(resp.Content, n)
	return dedupeQueries(query, variants), nil
}

// parseExpandJSON 从模型输出中解析字符串数组；兼容 markdown 代码块与松散格式。
func parseExpandJSON(raw string, n int) []string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimSpace(s)
		if j := strings.Index(s, "```"); j >= 0 {
			s = strings.TrimSpace(s[:j])
		}
	}
	if start := strings.Index(s, "["); start >= 0 {
		if end := strings.LastIndex(s, "]"); end > start {
			s = s[start : end+1]
		}
	}

	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return trimNonEmpty(arr, n)
	}

	// 回退：按行拆分
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "-*•0123456789.、)） ")
		line = strings.Trim(line, `"'「」`)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		lines = append(lines, line)
	}
	return trimNonEmpty(lines, n)
}

func trimNonEmpty(in []string, n int) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
		if n > 0 && len(out) >= n {
			break
		}
	}
	return out
}

// dedupeQueries 去掉与原问题相同（忽略空白大小写）的改写，并去重。
func dedupeQueries(original string, variants []string) []string {
	seen := map[string]struct{}{
		normalizeQueryKey(original): {},
	}
	out := make([]string, 0, len(variants))
	for _, v := range variants {
		k := normalizeQueryKey(v)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, strings.TrimSpace(v))
	}
	return out
}

func normalizeQueryKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

// buildExpandQueries 原问题 + 改写结果，用于多路召回。
func buildExpandQueries(original string, variants []string) []string {
	original = strings.TrimSpace(original)
	out := make([]string, 0, 1+len(variants))
	if original != "" {
		out = append(out, original)
	}
	out = append(out, variants...)
	return out
}
