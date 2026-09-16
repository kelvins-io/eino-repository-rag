package rag

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"
)

var (
	rePageHeader = regexp.MustCompile(`##\s*第\s*(\d+)\s*页`)
	reCitation   = regexp.MustCompile(`\[(\d+)\]`)
)

// inferPageFromContent 从解析器写入的「## 第 N 页」标记推断页码；无法识别返回 0
func inferPageFromContent(content string) int {
	m := rePageHeader.FindStringSubmatch(content)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func metaInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, err := strconv.Atoi(t)
		if err != nil {
			return 0
		}
		return n
	default:
		n, err := strconv.Atoi(fmt.Sprintf("%v", t))
		if err != nil {
			return 0
		}
		return n
	}
}

func buildSourceDocument(d *schema.Document) SourceDocument {
	if d == nil {
		return SourceDocument{}
	}
	docID := metaString(d.MetaData, "doc_id")
	chunkIndex := metaInt(d.MetaData, "chunk_index")
	page := metaInt(d.MetaData, "page")
	if page <= 0 {
		page = inferPageFromContent(d.Content)
	}
	return SourceDocument{
		ID:         d.ID,
		DocID:      docID,
		ChunkIndex: chunkIndex,
		Page:       page,
		Title:      metaString(d.MetaData, "title"),
		Section:    metaString(d.MetaData, "section"),
		Format:     metaString(d.MetaData, "format"),
		Content:    truncate(d.Content, 300),
		Score:      d.Score(),
	}
}

// CitationCheck 引用后校验结果
type CitationCheck struct {
	Answer     string // 清洗后的回答
	ValidCited []int  // 合法引用编号（1-based，升序去重）
	Removed    []int  // 越界/非法编号（升序去重）
	Changed    bool
}

// validateCitations 校验回答中的 [n] 是否落在 sources[1..sourceCount]；
// 越界引用删除；合法编号保留。sourceCount<=0 时去掉全部引用标记。
func validateCitations(answer string, sourceCount int) CitationCheck {
	out := CitationCheck{Answer: answer}
	if answer == "" || !strings.Contains(answer, "[") {
		return out
	}

	validSet := map[int]struct{}{}
	removedSet := map[int]struct{}{}

	cleaned := reCitation.ReplaceAllStringFunc(answer, func(m string) string {
		sub := reCitation.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		n, err := strconv.Atoi(sub[1])
		if err != nil || n <= 0 {
			removedSet[n] = struct{}{}
			return ""
		}
		if sourceCount <= 0 || n > sourceCount {
			removedSet[n] = struct{}{}
			return ""
		}
		validSet[n] = struct{}{}
		return m
	})

	// 清理因删除引用留下的多余空白
	cleaned = collapseCitationGaps(cleaned)
	out.Answer = cleaned
	out.ValidCited = sortedInts(validSet)
	out.Removed = sortedInts(removedSet)
	out.Changed = cleaned != answer || len(out.Removed) > 0
	return out
}

func collapseCitationGaps(s string) string {
	s = strings.ReplaceAll(s, "  ", " ")
	s = strings.ReplaceAll(s, " 。", "。")
	s = strings.ReplaceAll(s, " ，", "，")
	s = strings.ReplaceAll(s, " 、", "、")
	return strings.TrimSpace(s)
}

func sortedInts(set map[int]struct{}) []int {
	if len(set) == 0 {
		return nil
	}
	out := make([]int, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// filterSourcesByCited 仅保留被合法引用的 sources（1-based）；无引用时原样返回。
func filterSourcesByCited(sources []SourceDocument, cited []int) []SourceDocument {
	if len(cited) == 0 || len(sources) == 0 {
		return sources
	}
	keep := map[int]struct{}{}
	for _, n := range cited {
		keep[n] = struct{}{}
	}
	out := make([]SourceDocument, 0, len(cited))
	for i, s := range sources {
		if _, ok := keep[i+1]; ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return sources
	}
	return out
}
