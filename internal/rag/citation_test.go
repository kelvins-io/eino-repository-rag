package rag

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestInferPageFromContent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"header", "## 第 3 页\n正文内容", 3},
		{"compact", "##第12页\nsome text", 12},
		{"none", "没有页码标记", 0},
		{"zero", "## 第 0 页\nx", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferPageFromContent(tt.in); got != tt.want {
				t.Fatalf("inferPageFromContent()=%d want %d", got, tt.want)
			}
		})
	}
}

func TestBuildSourceDocument(t *testing.T) {
	d := &schema.Document{
		ID:      "chunk-1",
		Content: "## 第 5 页\n这是一段引用内容用于截断测试",
		MetaData: map[string]any{
			"doc_id":      "42",
			"chunk_index": 7,
			"title":       "报告.pdf",
			"format":      "pdf",
		},
	}
	d.WithScore(0.91)

	src := buildSourceDocument(d)
	if src.ID != "chunk-1" {
		t.Fatalf("ID=%q", src.ID)
	}
	if src.DocID != "42" {
		t.Fatalf("DocID=%q", src.DocID)
	}
	if src.ChunkIndex != 7 {
		t.Fatalf("ChunkIndex=%d", src.ChunkIndex)
	}
	if src.Page != 5 {
		t.Fatalf("Page=%d want 5 from content header", src.Page)
	}
	if src.Title != "报告.pdf" || src.Format != "pdf" {
		t.Fatalf("title/format = %q/%q", src.Title, src.Format)
	}
	if src.Score != 0.91 {
		t.Fatalf("Score=%v", src.Score)
	}
	if src.Content == "" {
		t.Fatal("Content empty")
	}
}

func TestBuildSourceDocumentPrefersMetaPage(t *testing.T) {
	d := &schema.Document{
		ID:      "c2",
		Content: "## 第 9 页\nbody",
		MetaData: map[string]any{
			"doc_id":  "1",
			"page":    2,
			"section": "第 2 页",
		},
	}
	src := buildSourceDocument(d)
	if src.Page != 2 {
		t.Fatalf("Page=%d want meta page 2", src.Page)
	}
	if src.Section != "第 2 页" {
		t.Fatalf("Section=%q", src.Section)
	}
}

func TestValidateCitations(t *testing.T) {
	ans := "根据规定不得内幕交易[1]，并须隔离[2]。另见[9]与[0]。"
	check := validateCitations(ans, 2)
	if !check.Changed {
		t.Fatal("expected changed")
	}
	if strings.Contains(check.Answer, "[9]") || strings.Contains(check.Answer, "[0]") {
		t.Fatalf("answer still has invalid cites: %q", check.Answer)
	}
	if !strings.Contains(check.Answer, "[1]") || !strings.Contains(check.Answer, "[2]") {
		t.Fatalf("lost valid cites: %q", check.Answer)
	}
	if len(check.ValidCited) != 2 || check.ValidCited[0] != 1 || check.ValidCited[1] != 2 {
		t.Fatalf("valid=%v", check.ValidCited)
	}
	if len(check.Removed) == 0 {
		t.Fatal("expected removed")
	}
}

func TestValidateCitationsNoSources(t *testing.T) {
	check := validateCitations("参考[1]说明", 0)
	if strings.Contains(check.Answer, "[1]") {
		t.Fatalf("should strip all cites: %q", check.Answer)
	}
}

func TestFilterSourcesByCited(t *testing.T) {
	sources := []SourceDocument{
		{ID: "a", Content: "A"},
		{ID: "b", Content: "B"},
		{ID: "c", Content: "C"},
	}
	got := filterSourcesByCited(sources, []int{1, 3})
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("%+v", got)
	}
	// 无引用时保留全部
	got2 := filterSourcesByCited(sources, nil)
	if len(got2) != 3 {
		t.Fatalf("%d", len(got2))
	}
}
