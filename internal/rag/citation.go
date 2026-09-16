package rag

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/cloudwego/eino/schema"
)

var rePageHeader = regexp.MustCompile(`##\s*第\s*(\d+)\s*页`)

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
		Format:     metaString(d.MetaData, "format"),
		Content:    truncate(d.Content, 300),
		Score:      d.Score(),
	}
}
