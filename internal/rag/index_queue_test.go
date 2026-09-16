package rag

import (
	"testing"
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
