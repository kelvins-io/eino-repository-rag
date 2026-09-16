package rag

import (
	"sort"

	"github.com/cloudwego/eino/schema"
)

// fuseRRF 对多路召回结果做 Reciprocal Rank Fusion。
// score(d) = Σ 1/(k + rank_i(d))，rank 从 1 开始；同 ID 文档合并内容（优先非空）。
func fuseRRF(lists [][]*schema.Document, k int) []*schema.Document {
	if k <= 0 {
		k = 60
	}
	type entry struct {
		doc   *schema.Document
		score float64
	}
	merged := make(map[string]*entry)

	for _, list := range lists {
		for rank, doc := range list {
			if doc == nil {
				continue
			}
			id := doc.ID
			if id == "" {
				id = metaString(doc.MetaData, "doc_id") + ":" + metaString(doc.MetaData, "chunk_index")
			}
			if id == "" {
				continue
			}
			rrf := 1.0 / float64(k+rank+1)
			if e, ok := merged[id]; ok {
				e.score += rrf
				if e.doc.Content == "" && doc.Content != "" {
					e.doc = doc
				}
			} else {
				cloned := cloneDoc(doc)
				merged[id] = &entry{doc: cloned, score: rrf}
			}
		}
	}

	out := make([]*schema.Document, 0, len(merged))
	for _, e := range merged {
		e.doc.WithScore(e.score)
		out = append(out, e.doc)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score() > out[j].Score()
	})
	return out
}

func truncateDocs(docs []*schema.Document, n int) []*schema.Document {
	if n <= 0 || len(docs) <= n {
		return docs
	}
	return docs[:n]
}

func cloneDoc(d *schema.Document) *schema.Document {
	if d == nil {
		return nil
	}
	out := &schema.Document{
		ID:      d.ID,
		Content: d.Content,
	}
	if d.MetaData != nil {
		out.MetaData = make(map[string]any, len(d.MetaData))
		for k, v := range d.MetaData {
			out.MetaData[k] = v
		}
	}
	return out
}

func candidateK(topK, configured int) int {
	if configured > 0 {
		return configured
	}
	if topK <= 0 {
		topK = 5
	}
	n := topK * 4
	if n < 10 {
		n = 10
	}
	return n
}
