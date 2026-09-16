package rag

import (
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
			"doc_id": "1",
			"page":   2,
		},
	}
	src := buildSourceDocument(d)
	if src.Page != 2 {
		t.Fatalf("Page=%d want meta page 2", src.Page)
	}
}
