package rag

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestScoreRetrievalHitAndMRR(t *testing.T) {
	c := GoldenCase{
		ID:               "c1",
		Query:            "合规红线",
		RelevantContains: []string{"不得利用未公开信息", "不得承诺客户投资收益"},
	}
	docs := []*schema.Document{
		{ID: "1", Content: "无关内容"},
		{ID: "2", Content: "从业人员不得利用未公开信息从事证券交易"},
		{ID: "3", Content: "不得承诺客户投资收益或分担损失"},
	}
	s := ScoreRetrieval(c, docs, 5)
	if !s.HitAtK {
		t.Fatal("expected hit")
	}
	if s.FirstRank != 2 {
		t.Fatalf("first_rank=%d", s.FirstRank)
	}
	if s.RR != 0.5 {
		t.Fatalf("rr=%v", s.RR)
	}
	if s.MatchedN != 2 {
		t.Fatalf("matched=%d", s.MatchedN)
	}
	if s.RecallAtK != 1.0 {
		t.Fatalf("recall=%v", s.RecallAtK)
	}
}

func TestScoreRetrievalDocIDAndTitle(t *testing.T) {
	c := GoldenCase{
		ID:             "c2",
		Query:          "隔离墙",
		RelevantDocIDs: []string{"42"},
		RelevantTitles: []string{"合规"},
	}
	docs := []*schema.Document{
		{
			ID:       "x",
			Content:  "信息隔离墙",
			MetaData: map[string]any{"doc_id": "42", "title": "证券合规红线手册"},
		},
	}
	s := ScoreRetrieval(c, docs, 3)
	if !s.HitAtK || s.MatchedN != 2 {
		t.Fatalf("score=%+v", s)
	}
}

func TestAggregateScores(t *testing.T) {
	scores := []CaseScore{
		{HitAtK: true, RecallAtK: 1, RR: 1},
		{HitAtK: false, RecallAtK: 0, RR: 0},
		{Error: "fail", HitAtK: true, RR: 1}, // skipped
	}
	rep := AggregateScores(scores, 5)
	if rep.Scored != 2 {
		t.Fatalf("scored=%d", rep.Scored)
	}
	if rep.HitAtK != 0.5 {
		t.Fatalf("hit=%v", rep.HitAtK)
	}
	if rep.MRR != 0.5 {
		t.Fatalf("mrr=%v", rep.MRR)
	}
}

func TestLoadGoldenJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "golden.jsonl")
	content := `# comment
{"id":"a","query":"红线","relevant_contains":["未公开信息"]}
{"id":"b","query":"适当性","relevant_titles":["适当性"]}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, err := LoadGoldenJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("len=%d", len(cases))
	}
}

func TestLoadGoldenJSONLRejectsEmptyCriteria(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(path, []byte(`{"id":"x","query":"q"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGoldenJSONL(path); err == nil {
		t.Fatal("expected error")
	}
}
