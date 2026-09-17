package rag

import (
	"testing"
	"time"
)

func TestParseIndexJob(t *testing.T) {
	job, err := parseIndexJob(`{"doc_id":42,"attempt":2,"error":"boom"}`)
	if err != nil {
		t.Fatal(err)
	}
	if job.DocID != 42 || job.Attempt != 2 || job.Error != "boom" {
		t.Fatalf("unexpected job: %+v", job)
	}

	if _, err := parseIndexJob(`{}`); err == nil {
		t.Fatal("expected error for missing doc_id")
	}
	if _, err := parseIndexJob(`not-json`); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestIndexRetryBackoff(t *testing.T) {
	if got := indexRetryBackoff(1, 5); got != 5*time.Second {
		t.Fatalf("attempt 1: got %s", got)
	}
	if got := indexRetryBackoff(2, 5); got != 10*time.Second {
		t.Fatalf("attempt 2: got %s", got)
	}
	if got := indexRetryBackoff(3, 5); got != 20*time.Second {
		t.Fatalf("attempt 3: got %s", got)
	}
}
