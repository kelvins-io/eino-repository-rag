package rag

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// GoldenCase 检索评测样例（JSONL 一行一条）。
// 相关性判定（任一命中即可视为该文档相关）：
//   - relevant_doc_ids：chunk meta doc_id
//   - relevant_titles：title 子串匹配
//   - relevant_contains：content 子串匹配（适合示例库未绑定固定 doc_id）
type GoldenCase struct {
	ID               string   `json:"id"`
	Query            string   `json:"query"`
	RelevantDocIDs   []string `json:"relevant_doc_ids,omitempty"`
	RelevantTitles   []string `json:"relevant_titles,omitempty"`
	RelevantContains []string `json:"relevant_contains,omitempty"`
	KnowledgeBaseID  uint     `json:"knowledge_base_id,omitempty"`
	DirectoryID      *uint    `json:"directory_id,omitempty"`
	TenantID         uint     `json:"tenant_id,omitempty"`
}

// CaseScore 单条评测结果
type CaseScore struct {
	ID        string  `json:"id"`
	Query     string  `json:"query"`
	HitAtK    bool    `json:"hit_at_k"`
	RecallAtK float64 `json:"recall_at_k"`
	RR        float64 `json:"rr"` // reciprocal rank；未命中为 0
	FirstRank int     `json:"first_rank,omitempty"`
	Retrieved int     `json:"retrieved"`
	ExpectedN int     `json:"expected_n"`
	MatchedN  int     `json:"matched_n"`
	Error     string  `json:"error,omitempty"`
}

// EvalReport 汇总报告
type EvalReport struct {
	K          int         `json:"k"`
	Cases      int         `json:"cases"`
	Scored     int         `json:"scored"`
	HitAtK     float64     `json:"hit_at_k"`
	RecallAtK  float64     `json:"recall_at_k"`
	MRR        float64     `json:"mrr"`
	CaseScores []CaseScore `json:"case_scores"`
}

// LoadGoldenJSONL 加载 JSONL 评测集
func LoadGoldenJSONL(path string) ([]GoldenCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cases []GoldenCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var c GoldenCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("golden line %d: %w", lineNo, err)
		}
		if strings.TrimSpace(c.Query) == "" {
			return nil, fmt.Errorf("golden line %d: empty query", lineNo)
		}
		if c.ID == "" {
			c.ID = fmt.Sprintf("case-%d", lineNo)
		}
		if err := validateGoldenCase(c); err != nil {
			return nil, fmt.Errorf("golden line %d (%s): %w", lineNo, c.ID, err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("golden set is empty: %s", path)
	}
	return cases, nil
}

func validateGoldenCase(c GoldenCase) error {
	if len(c.RelevantDocIDs) == 0 && len(c.RelevantTitles) == 0 && len(c.RelevantContains) == 0 {
		return fmt.Errorf("need at least one of relevant_doc_ids / relevant_titles / relevant_contains")
	}
	return nil
}

// ScoreRetrieval 对单条召回结果打分（K = topK）
func ScoreRetrieval(c GoldenCase, docs []*schema.Document, k int) CaseScore {
	if k <= 0 {
		k = 5
	}
	top := truncateDocs(docs, k)
	expectedN := expectedCriteriaCount(c)
	cs := CaseScore{
		ID:        c.ID,
		Query:     c.Query,
		Retrieved: len(top),
		ExpectedN: expectedN,
	}

	firstRank := 0
	matchedCriteria := map[string]struct{}{}
	for i, d := range top {
		keys := matchCriteria(c, d)
		if len(keys) == 0 {
			continue
		}
		if firstRank == 0 {
			firstRank = i + 1
		}
		for _, key := range keys {
			matchedCriteria[key] = struct{}{}
		}
	}

	cs.MatchedN = len(matchedCriteria)
	cs.HitAtK = firstRank > 0
	if firstRank > 0 {
		cs.FirstRank = firstRank
		cs.RR = 1.0 / float64(firstRank)
	}
	if expectedN > 0 {
		cs.RecallAtK = float64(cs.MatchedN) / float64(expectedN)
		if cs.RecallAtK > 1 {
			cs.RecallAtK = 1
		}
	}
	return cs
}

func expectedCriteriaCount(c GoldenCase) int {
	n := 0
	n += len(c.RelevantDocIDs)
	n += len(c.RelevantTitles)
	n += len(c.RelevantContains)
	return n
}

// matchCriteria 返回本 chunk 命中的期望条件 key（用于 Recall 去重计数）
func matchCriteria(c GoldenCase, d *schema.Document) []string {
	if d == nil {
		return nil
	}
	var keys []string
	docID := metaString(d.MetaData, "doc_id")
	title := metaString(d.MetaData, "title")
	content := d.Content

	for _, id := range c.RelevantDocIDs {
		if id != "" && docID == id {
			keys = append(keys, "doc_id:"+id)
		}
	}
	for _, t := range c.RelevantTitles {
		t = strings.TrimSpace(t)
		if t != "" && strings.Contains(title, t) {
			keys = append(keys, "title:"+t)
		}
	}
	for _, sub := range c.RelevantContains {
		sub = strings.TrimSpace(sub)
		if sub != "" && strings.Contains(content, sub) {
			keys = append(keys, "contains:"+sub)
		}
	}
	return keys
}

// AggregateScores 汇总多条 CaseScore
func AggregateScores(scores []CaseScore, k int) EvalReport {
	rep := EvalReport{K: k, Cases: len(scores), CaseScores: scores}
	if len(scores) == 0 {
		return rep
	}
	var hitSum, recallSum, mrrSum float64
	scored := 0
	for _, s := range scores {
		if s.Error != "" {
			continue
		}
		scored++
		if s.HitAtK {
			hitSum++
		}
		recallSum += s.RecallAtK
		mrrSum += s.RR
	}
	rep.Scored = scored
	if scored > 0 {
		rep.HitAtK = hitSum / float64(scored)
		rep.RecallAtK = recallSum / float64(scored)
		rep.MRR = mrrSum / float64(scored)
	}
	return rep
}

// FilterFromGolden 由评测样例组装检索过滤条件
func FilterFromGolden(c GoldenCase) *RetrieveFilter {
	f := &RetrieveFilter{}
	if c.TenantID > 0 {
		f.TenantID = uintToMeta(c.TenantID)
	}
	if c.KnowledgeBaseID > 0 {
		f.KnowledgeBaseID = uintToMeta(c.KnowledgeBaseID)
	}
	if c.DirectoryID != nil {
		f.DirectoryIDs = []string{uintToMeta(*c.DirectoryID)}
	}
	return f
}
