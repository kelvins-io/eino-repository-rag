package rag

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/rag/parser"
)

func TestHandleJobInterruptedSkipsProcess(t *testing.T) {
	q := &IndexQueue{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// parent 已取消时应直接返回，不得调用 process（pipe/rdb 均为 nil）
	q.handleJob(ctx, `{"doc_id":1,"attempt":0}`)
}

func TestStopWaitsForWorker(t *testing.T) {
	q := &IndexQueue{started: true}
	ctx, cancel := context.WithCancel(context.Background())
	q.runCtx, q.cancel = ctx, cancel
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		<-ctx.Done()
	}()
	q.Stop()
	if q.started {
		t.Fatal("expected started=false")
	}
}

func TestStopTimesOutWhenWorkerStuck(t *testing.T) {
	q := &IndexQueue{started: true, stopTimeout: 50 * time.Millisecond}
	q.runCtx, q.cancel = context.WithCancel(context.Background())
	q.wg.Add(1) // 永不 Done，模拟卡在无 ctx 的索引任务里
	done := make(chan struct{})
	go func() {
		q.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop hung")
	}
	if q.started {
		t.Fatal("expected started=false after timeout")
	}
	q.wg.Done() // 释放 Stop 里 wg.Wait 的 goroutine
}

func TestFailDocumentSkipsWhenCanceled(t *testing.T) {
	p := &Pipeline{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.failDocument(ctx, 1, "boom") // ctx 已取消且 docRepo 为 nil，不得 panic
}

func TestWaitGroupTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	if waitGroupTimeout(&wg, 30*time.Millisecond) {
		t.Fatal("expected timeout")
	}
	wg.Done()
	if !waitGroupTimeout(&wg, time.Second) {
		t.Fatal("expected wait to complete")
	}
}

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
