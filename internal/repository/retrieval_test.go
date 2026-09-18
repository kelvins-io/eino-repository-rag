package repository

import "testing"

func TestUniqueNumericDocIDs(t *testing.T) {
	got := uniqueNumericDocIDs([]string{" 12 ", "12", "0", "abc", "3"})
	if len(got) != 2 || got[0] != "12" || got[1] != "3" {
		t.Fatalf("%v", got)
	}
}

func TestRecallRatio(t *testing.T) {
	if _, ok := RecallRatio(0, 0); ok {
		t.Fatal("expected undefined recall when nothing is labeled")
	}
	got, ok := RecallRatio(4, 1)
	if !ok || got != 0.25 {
		t.Fatalf("recall=%v ok=%v", got, ok)
	}
	got, ok = RecallRatio(2, 5)
	if !ok || got != 1 {
		t.Fatalf("clamped recall=%v ok=%v", got, ok)
	}
}

func TestRankCitedChunks(t *testing.T) {
	got := RankCitedChunks(nil)
	if len(got) != 0 {
		t.Fatalf("empty=%v", got)
	}
	got = RankCitedChunks(map[int]int{3: 2, 0: 1, 1: 2, 5: 9, 8: 1, -1: 4, 4: 0})
	if len(got) != 3 {
		t.Fatalf("len=%d %#v", len(got), got)
	}
	if got[0].Rank != 1 || got[0].ChunkIndex != 5 || got[0].Count != 9 {
		t.Fatalf("first=%#v", got[0])
	}
	if got[1].Rank != 2 || got[1].ChunkIndex != 1 || got[1].Count != 2 {
		t.Fatalf("second=%#v", got[1])
	}
	if got[2].Rank != 3 || got[2].ChunkIndex != 3 || got[2].Count != 2 {
		t.Fatalf("third=%#v", got[2])
	}
}
