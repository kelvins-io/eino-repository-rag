package rag

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/embedding"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

type stubEmbedder struct {
	mu        sync.Mutex
	calls     [][]string
	failTimes int
	err       error
	delay     time.Duration
}

func (s *stubEmbedder) EmbedStrings(ctx context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.delay):
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, append([]string(nil), texts...))
	if s.failTimes > 0 {
		s.failTimes--
		return nil, s.err
	}
	out := make([][]float64, len(texts))
	for i := range texts {
		out[i] = []float64{float64(len(texts[i]))}
	}
	return out, nil
}

func TestResilientEmbedderSplitsBatches(t *testing.T) {
	stub := &stubEmbedder{}
	emb := newResilientEmbedder(stub, config.EmbeddingConfig{
		BatchSize:      2,
		MaxRetries:     0,
		MaxConcurrency: 1,
	}).(*resilientEmbedder)
	emb.retryBackoff = 0

	got, err := emb.EmbedStrings(context.Background(), []string{"a", "bb", "ccc", "dddd", "eeeee"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 vectors, got %d", len(got))
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.calls) != 3 {
		t.Fatalf("expected 3 batches, got %d: %v", len(stub.calls), stub.calls)
	}
	if len(stub.calls[0]) != 2 || len(stub.calls[1]) != 2 || len(stub.calls[2]) != 1 {
		t.Fatalf("unexpected batch sizes: %v", stub.calls)
	}
}

func TestResilientEmbedderRetriesTimeout(t *testing.T) {
	timeoutErr := &url.Error{
		Op:  "Post",
		URL: "https://api.siliconflow.cn/v1/embeddings",
		Err: errors.New("context deadline exceeded (Client.Timeout exceeded while awaiting headers)"),
	}
	stub := &stubEmbedder{failTimes: 2, err: timeoutErr}
	emb := newResilientEmbedder(stub, config.EmbeddingConfig{
		BatchSize:      10,
		MaxRetries:     2,
		MaxConcurrency: 1,
	}).(*resilientEmbedder)
	emb.retryBackoff = time.Millisecond

	got, err := emb.EmbedStrings(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(got))
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.calls) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(stub.calls))
	}
}

func TestResilientEmbedderNoRetryOnAuthError(t *testing.T) {
	stub := &stubEmbedder{failTimes: 5, err: errors.New("401 invalid api key")}
	emb := newResilientEmbedder(stub, config.EmbeddingConfig{
		BatchSize:      10,
		MaxRetries:     3,
		MaxConcurrency: 1,
	}).(*resilientEmbedder)
	emb.retryBackoff = time.Millisecond

	_, err := emb.EmbedStrings(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected auth error")
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(stub.calls))
	}
}

func TestResilientEmbedderRespectsCancel(t *testing.T) {
	stub := &stubEmbedder{failTimes: 1, err: context.DeadlineExceeded}
	emb := newResilientEmbedder(stub, config.EmbeddingConfig{
		BatchSize:      10,
		MaxRetries:     3,
		MaxConcurrency: 1,
	}).(*resilientEmbedder)
	emb.retryBackoff = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := emb.EmbedStrings(ctx, []string{"hello"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestIsRetryableEmbedErr(t *testing.T) {
	timeout := errors.New(`Post "https://api.siliconflow.cn/v1/embeddings": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`)
	if !isRetryableEmbedErr(timeout) {
		t.Fatal("timeout should be retryable")
	}
	if !isRetryableEmbedErr(errors.New("429 too many requests")) {
		t.Fatal("429 should be retryable")
	}
	if isRetryableEmbedErr(errors.New("invalid api key")) {
		t.Fatal("auth error should not be retryable")
	}
}
