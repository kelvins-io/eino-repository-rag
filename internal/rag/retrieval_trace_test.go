package rag

import (
	"testing"

	"github.com/cloudwego/eino/schema"

	dbmodel "github.com/kelvins-io/eino-repository-rag/internal/model"
)

func TestLinearRetrievalHitsUsesCitationIndices(t *testing.T) {
	sources := []SourceDocument{
		{DocID: "9", ChunkIndex: 0, Content: "a"},
		{DocID: " 12 ", ChunkIndex: 2, Content: "b"},
		{DocID: "", ChunkIndex: 1, Content: "skip"},
		{DocID: "3", ChunkIndex: 4, Content: "c"},
	}
	hits := LinearRetrievalHits(sources, []int{2})
	if len(hits) != 3 {
		t.Fatalf("hits=%d", len(hits))
	}
	if hits[0].DocID != "9" || hits[0].Rank != 1 || hits[0].Cited || hits[0].Round != 1 {
		t.Fatalf("first=%+v", hits[0])
	}
	if hits[1].DocID != "12" || hits[1].Rank != 2 || !hits[1].Cited || hits[1].ChunkIndex != 2 {
		t.Fatalf("cited=%+v", hits[1])
	}
	if hits[2].DocID != "3" || hits[2].Rank != 4 || hits[2].Cited {
		t.Fatalf("rank should follow source index, got %+v", hits[2])
	}
}

func TestAgentRetrievalHitsKeepRoundRankAndGlobalCitation(t *testing.T) {
	a := &schema.Document{ID: "a", MetaData: map[string]any{"doc_id": "1", "chunk_index": 0}}
	b := &schema.Document{ID: "b", MetaData: map[string]any{"doc_id": "2", "chunk_index": 1}}
	c := &schema.Document{ID: "c", MetaData: map[string]any{"doc_id": "3", "chunk_index": 0}}
	rounds := [][]*schema.Document{
		{a, b},
		{b, c},
	}
	global := map[string]int{"a": 1, "b": 2, "c": 3}
	hits := AgentRetrievalHits(rounds, func(d *schema.Document) int {
		return global[d.ID]
	}, []int{3})
	if len(hits) != 4 {
		t.Fatalf("hits=%d", len(hits))
	}
	if hits[0].Round != 1 || hits[0].Rank != 1 || hits[0].DocID != "1" || hits[0].Cited {
		t.Fatalf("round1 first=%+v", hits[0])
	}
	if hits[2].Round != 2 || hits[2].Rank != 1 || hits[2].DocID != "2" || hits[2].Cited {
		t.Fatalf("round2 should keep original rank, got %+v", hits[2])
	}
	if hits[3].Round != 2 || hits[3].Rank != 2 || hits[3].DocID != "3" || !hits[3].Cited {
		t.Fatalf("citation must use global index, got %+v", hits[3])
	}
}

type fakeHitWriter struct {
	hits []dbmodel.RetrievalHit
}

func (f *fakeHitWriter) CreateHits(hits []dbmodel.RetrievalHit) error {
	f.hits = append(f.hits, hits...)
	return nil
}

func TestPersistRetrievalHitsAttachesMessages(t *testing.T) {
	w := &fakeHitWriter{}
	p := &Pipeline{retrieval: w}
	hits := LinearRetrievalHits([]SourceDocument{{DocID: "7", ChunkIndex: 1}}, []int{1})
	p.persistRetrievalHits(QueryRequest{
		TenantID:        3,
		UserID:          "u",
		SessionID:       "s",
		KnowledgeBaseID: 8,
	}, &dbmodel.Message{ID: 11}, &dbmodel.Message{ID: 12}, hits)
	if len(w.hits) != 1 {
		t.Fatalf("saved=%d", len(w.hits))
	}
	got := w.hits[0]
	if got.UserMessageID != 11 || got.AssistantMessageID != 12 || got.DocID != "7" || !got.Cited || got.KnowledgeBaseID != 8 {
		t.Fatalf("%+v", got)
	}
}
