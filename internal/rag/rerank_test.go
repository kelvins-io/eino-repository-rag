package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

func TestHTTPReranker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rerank" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		var req rerankRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(rerankResponse{
			Results: []rerankResult{
				{Index: 1, RelevanceScore: 0.9},
				{Index: 0, RelevanceScore: 0.5},
			},
		})
	}))
	defer srv.Close()

	rr := newHTTPReranker(config.RerankConfig{
		Enabled:        true,
		APIKey:         "test",
		Model:          "BAAI/bge-reranker-v2-m3",
		BaseURL:        srv.URL,
		TopN:           2,
		TimeoutSeconds: 5,
	})
	docs := []*schema.Document{
		{ID: "a", Content: "first"},
		{ID: "b", Content: "second"},
	}
	out, err := rr.Rerank(context.Background(), "q", docs, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].ID != "b" {
		t.Fatalf("want b first, got %s", out[0].ID)
	}
}
