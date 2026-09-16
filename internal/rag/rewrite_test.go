package rag

import (
	"strings"
	"testing"
)

func TestParseExpandJSON(t *testing.T) {
	raw := `["合规红线有哪些","从业人员禁止行为"]`
	got := parseExpandJSON(raw, 2)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestParseExpandJSONMarkdown(t *testing.T) {
	raw := "```json\n[\"问法A\", \"问法B\", \"问法C\"]\n```"
	got := parseExpandJSON(raw, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
	if got[0] != "问法A" {
		t.Fatalf("got %q", got[0])
	}
}

func TestDedupeQueries(t *testing.T) {
	got := dedupeQueries("合规红线", []string{"合规红线", "  合规红线  ", "禁止行为", "禁止行为"})
	if len(got) != 1 || got[0] != "禁止行为" {
		t.Fatalf("got %v", got)
	}
}

func TestBuildExpandQueries(t *testing.T) {
	got := buildExpandQueries("原问题", []string{"改写1"})
	if len(got) != 2 || got[0] != "原问题" || got[1] != "改写1" {
		t.Fatalf("got %v", got)
	}
}

func TestParseExpandJSONFallbackLines(t *testing.T) {
	raw := "1. 适当性评估流程\n2. 投资者风险等级\n"
	got := parseExpandJSON(raw, 2)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(got[0], "适当性") {
		t.Fatalf("got %q", got[0])
	}
}
