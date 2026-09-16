package rag

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestFuseRRF(t *testing.T) {
	dense := []*schema.Document{
		{ID: "a", Content: "A"},
		{ID: "b", Content: "B"},
		{ID: "c", Content: "C"},
	}
	sparse := []*schema.Document{
		{ID: "c", Content: "C"},
		{ID: "a", Content: "A"},
		{ID: "d", Content: "D"},
	}
	fused := fuseRRF([][]*schema.Document{dense, sparse}, 60)
	if len(fused) != 4 {
		t.Fatalf("expected 4 docs, got %d", len(fused))
	}
	// a: 1/(60+1)+1/(60+2)=1/61+1/62
	// c: 1/(60+3)+1/(60+1)=1/63+1/61  -> c slightly higher than a?
	// rank dense: a=1,b=2,c=3; sparse: c=1,a=2,d=3
	// a: 1/61 + 1/62
	// c: 1/63 + 1/61
	// a > c because 1/62 > 1/63
	if fused[0].ID != "a" {
		t.Fatalf("expected first id=a, got %s score=%v", fused[0].ID, fused[0].Score())
	}
	if fused[1].ID != "c" {
		t.Fatalf("expected second id=c, got %s", fused[1].ID)
	}
	if fused[0].Score() <= fused[1].Score() {
		t.Fatalf("expected a score > c score")
	}
}

func TestTokenizeQuery(t *testing.T) {
	toks := tokenizeQuery("适当性管理办法 Article12")
	if len(toks) == 0 {
		t.Fatal("expected tokens")
	}
	if !contains(toks, "article12") {
		t.Fatalf("expected article12 in %v", toks)
	}
	hasCJK := false
	for _, tkn := range toks {
		for _, r := range tkn {
			if r >= 0x4e00 {
				hasCJK = true
				break
			}
		}
	}
	if !hasCJK {
		t.Fatalf("expected CJK tokens in %v", toks)
	}
}

func TestBuildRedisTextQuery(t *testing.T) {
	q := buildRedisTextQuery("合规 12条")
	if q == "" {
		t.Fatal("empty query")
	}
	if q[:1] != "@" {
		t.Fatalf("unexpected query: %s", q)
	}
}

func TestCandidateK(t *testing.T) {
	if candidateK(5, 0) != 20 {
		t.Fatalf("expected 20, got %d", candidateK(5, 0))
	}
	if candidateK(5, 30) != 30 {
		t.Fatalf("expected 30, got %d", candidateK(5, 30))
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestTruncateDocs(t *testing.T) {
	docs := []*schema.Document{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	out := truncateDocs(docs, 2)
	if len(out) != 2 {
		t.Fatalf("got %d", len(out))
	}
}

func TestBuildRedisFilterUserID(t *testing.T) {
	q := buildRedisFilter(&RetrieveFilter{
		UserID:          "u001",
		KnowledgeBaseID: "1",
		DirectoryIDs:    []string{"2", "3"},
	})
	if q == "" {
		t.Fatal("empty filter")
	}
	if !strings.Contains(q, "@user_id:{u001}") {
		t.Fatalf("missing user_id in %q", q)
	}
	if !strings.Contains(q, "@kb_id:{1}") {
		t.Fatalf("missing kb_id in %q", q)
	}
	if !strings.Contains(q, "@directory_id:{2|3}") {
		t.Fatalf("missing directory_id in %q", q)
	}
}

func TestBuildMilvusFilterUserID(t *testing.T) {
	expr := buildMilvusFilter(&RetrieveFilter{
		UserID:          "u001",
		KnowledgeBaseID: "12",
	})
	if !strings.Contains(expr, `metadata["user_id"] == "u001"`) {
		t.Fatalf("missing user_id in %q", expr)
	}
	if !strings.Contains(expr, `metadata["kb_id"] == "12"`) {
		t.Fatalf("missing kb_id in %q", expr)
	}
}
