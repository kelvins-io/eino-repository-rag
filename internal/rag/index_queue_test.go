package rag

import (
	"fmt"
	"testing"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/rag/parser"
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

func TestIsRetryableIndexErr(t *testing.T) {
	if isRetryableIndexErr(errDocGone) {
		t.Fatal("doc gone should not retry")
	}
	_, err := parser.ExtractFile("old.doc", "")
	if err == nil || !parser.IsPermanent(err) {
		t.Fatalf("expected permanent .doc error, got %v", err)
	}
	if isRetryableIndexErr(fmt.Errorf("parse file: %w", err)) {
		t.Fatal("wrapped permanent parse error should not retry")
	}
	if isRetryableIndexErr(fmt.Errorf("parse file: pdf 未提取到文本")) {
		t.Fatal("parse errors should not retry")
	}
	if isRetryableIndexErr(fmt.Errorf("split document: boom")) {
		t.Fatal("split errors should not retry")
	}
	if !isRetryableIndexErr(fmt.Errorf("milvus store vectors: context deadline exceeded")) {
		t.Fatal("embed/milvus timeout should retry")
	}
}
