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
