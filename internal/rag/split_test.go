package rag

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

type fakeRecursive struct{}

func (fakeRecursive) Transform(_ context.Context, docs []*schema.Document, _ ...document.TransformerOption) ([]*schema.Document, error) {
	var out []*schema.Document
	for _, d := range docs {
		parts := strings.Split(d.Content, "\n\n")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, &schema.Document{Content: p})
			}
		}
	}
	return out, nil
}

func TestSplitByPageMarkers(t *testing.T) {
	text := "## 第 1 页\n\n第一页内容AAA\n\n## 第 2 页\n\n第二页内容BBB"
	secs := splitByPageMarkers(text)
	if len(secs) != 2 {
		t.Fatalf("secs=%d", len(secs))
	}
	if secs[0].Page != 1 || !strings.Contains(secs[0].Content, "AAA") {
		t.Fatalf("%+v", secs[0])
	}
	if secs[1].Page != 2 || !strings.Contains(secs[1].Content, "BBB") {
		t.Fatalf("%+v", secs[1])
	}
}

func TestSplitBySheetMarkers(t *testing.T) {
	text := "## 工作表: 汇总\n\na\tb\n\n## 工作表: 明细\n\nx\ty"
	secs := splitBySheetMarkers(text)
	if len(secs) != 2 {
		t.Fatalf("secs=%d %+v", len(secs), secs)
	}
	if secs[0].Title != "工作表: 汇总" {
		t.Fatalf("title=%q", secs[0].Title)
	}
}

func TestSplitByHeadings(t *testing.T) {
	text := "# 合规红线\n\n不得内幕交易\n\n## 隔离墙\n\n跨墙须审批"
	secs := splitByHeadings(text)
	if len(secs) < 2 {
		t.Fatalf("expected >=2 sections, got %d %+v", len(secs), secs)
	}
	if secs[0].Title != "合规红线" {
		t.Fatalf("title0=%q", secs[0].Title)
	}
}

func TestSplitDocumentPDFKeepsPageMeta(t *testing.T) {
	text := "## 第 1 页\n\n短内容一\n\n## 第 3 页\n\n短内容三"
	docs, err := splitDocument(context.Background(), fakeRecursive{}, text, "pdf", 800, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("docs=%d", len(docs))
	}
	if metaInt(docs[0].MetaData, "page") != 1 {
		t.Fatalf("page0=%v meta=%v", metaInt(docs[0].MetaData, "page"), docs[0].MetaData)
	}
	if metaInt(docs[1].MetaData, "page") != 3 {
		t.Fatalf("page1=%v", metaInt(docs[1].MetaData, "page"))
	}
	if metaString(docs[0].MetaData, "section") != "第 1 页" {
		t.Fatalf("section=%q", metaString(docs[0].MetaData, "section"))
	}
}

func TestSplitDocumentDisabledFallsBack(t *testing.T) {
	text := "# A\n\nbodyA\n\n# B\n\nbodyB"
	docs, err := splitDocument(context.Background(), fakeRecursive{}, text, "markdown", 800, false)
	if err != nil {
		t.Fatal(err)
	}
	// disabled → 整篇一条（未超 chunk）
	if len(docs) != 1 {
		t.Fatalf("docs=%d", len(docs))
	}
}

func TestSplitByStructureMarkdown(t *testing.T) {
	text := "# 标题甲\n\n内容甲\n\n# 标题乙\n\n内容乙"
	secs := splitByStructure(text, "markdown")
	if len(secs) != 2 {
		t.Fatalf("secs=%d", len(secs))
	}
}
