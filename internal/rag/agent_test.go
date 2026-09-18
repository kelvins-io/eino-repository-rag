package rag

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestAgentDocCollectorMerge(t *testing.T) {
	c := newAgentDocCollector()
	d1 := &schema.Document{ID: "a", Content: "A"}
	d2 := &schema.Document{ID: "b", Content: "B"}
	_, idx1 := c.Merge([]*schema.Document{d1, d2})
	if len(idx1) != 2 || idx1[0] != 1 || idx1[1] != 2 {
		t.Fatalf("idx1=%v", idx1)
	}
	_, idx2 := c.Merge([]*schema.Document{d2, {ID: "c", Content: "C"}})
	if len(idx2) != 2 || idx2[0] != 2 || idx2[1] != 3 {
		t.Fatalf("idx2=%v", idx2)
	}
	docs := c.Docs()
	if len(docs) != 3 {
		t.Fatalf("docs=%d", len(docs))
	}
}

func TestAgentDocCollectorRecordRoundKeepsRetrieveOrder(t *testing.T) {
	c := newAgentDocCollector()
	first := &schema.Document{ID: "a"}
	second := &schema.Document{ID: "b"}
	again := &schema.Document{ID: "b"}
	third := &schema.Document{ID: "c"}
	c.RecordRound([]*schema.Document{first, second})
	c.Merge([]*schema.Document{first, second})
	c.RecordRound([]*schema.Document{again, third})
	c.Merge([]*schema.Document{again, third})

	rounds := c.Rounds()
	if len(rounds) != 2 || len(rounds[1]) != 2 || rounds[1][0].ID != "b" || rounds[1][1].ID != "c" {
		t.Fatalf("rounds lost original order: %+v", rounds)
	}
	if c.GlobalIndex(third) != 3 || c.GlobalIndex(again) != 2 {
		t.Fatalf("global=%d %d", c.GlobalIndex(again), c.GlobalIndex(third))
	}
}

func TestParseToolQueryFromArgs(t *testing.T) {
	q := parseToolQueryFromArgs(`{"query":"合规红线"}`)
	if q != "合规红线" {
		t.Fatalf("q=%q", q)
	}
	if parseToolQueryFromArgs("not-json") != "" {
		t.Fatal("expected empty")
	}
}

func TestFormatRetrieveSummary(t *testing.T) {
	docs := []*schema.Document{
		{
			ID:       "1",
			Content:  "不得内幕交易",
			MetaData: map[string]any{"doc_id": "9", "title": "合规", "page": 1},
		},
	}
	s := formatRetrieveSummary(docs, []int{3})
	if !strings.Contains(s, "[3]") || !strings.Contains(s, "不得内幕交易") {
		t.Fatalf("summary=%q", s)
	}
}

func TestBuildAgentInputMessages(t *testing.T) {
	msgs := buildAgentInputMessages("你好", nil)
	if len(msgs) != 1 || msgs[0].Content != "你好" {
		t.Fatalf("%+v", msgs)
	}
}

func TestMergeDocsByID(t *testing.T) {
	a := []*schema.Document{{ID: "1", Content: "a"}, {ID: "2", Content: "b"}}
	b := []*schema.Document{{ID: "2", Content: "b2"}, {ID: "3", Content: "c"}}
	got := mergeDocsByID(a, b)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[1].Content != "b" {
		t.Fatalf("should keep first: %q", got[1].Content)
	}
}
