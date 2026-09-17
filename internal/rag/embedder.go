package rag

import (
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/embedding"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

const (
	defaultEmbedBatchSize      = 16
	defaultEmbedMaxConcurrency = 2
	defaultEmbedRetryBackoff   = time.Second
	maxEmbedRetryBackoff       = 16 * time.Second
)

// resilientEmbedder 在 OpenAI 兼容 Embedder 外包一层：拆批、限流、瞬时错误重试。
// Milvus Indexer 会把整篇文档一次性 EmbedStrings，SiliconFlow 对大批量请求容易
// Client.Timeout exceeded while awaiting headers。
type resilientEmbedder struct {
	inner        embedding.Embedder
	batchSize    int
	maxRetries   int
	sem          chan struct{}
	retryBackoff time.Duration
}

func newResilientEmbedder(inner embedding.Embedder, cfg config.EmbeddingConfig) embedding.Embedder {
	if inner == nil {
		return nil
	}
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = defaultEmbedBatchSize
	}
	retries := cfg.MaxRetries
	if retries < 0 {
		retries = 0
	}
	conc := cfg.MaxConcurrency
	if conc <= 0 {
		conc = defaultEmbedMaxConcurrency
	}
	return &resilientEmbedder{
		inner:        inner,
		batchSize:    batch,
		maxRetries:   retries,
		sem:          make(chan struct{}, conc),
		retryBackoff: defaultEmbedRetryBackoff,
	}
}

func (e *resilientEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if len(texts) > e.batchSize {
		log.Printf("[rag] embed split texts=%d batch_size=%d", len(texts), e.batchSize)
	}
	out := make([][]float64, 0, len(texts))
	for i := 0; i < len(texts); i += e.batchSize {
		end := i + e.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := e.embedBatch(ctx, texts[i:end], opts...)
		if err != nil {
			return nil, err
		}
		if len(batch) != end-i {
			return nil, errors.New("embed batch result length mismatch")
		}
		out = append(out, batch...)
	}
	return out, nil
}

func (e *resilientEmbedder) embedBatch(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	var lastErr error
	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			if err := waitEmbedRetry(ctx, attempt-1, e.retryBackoff); err != nil {
				return nil, err
			}
			log.Printf("[rag] embed batch retry attempt=%d/%d n=%d err=%v",
				attempt, e.maxRetries, len(texts), lastErr)
		}

		vecs, err := e.call(ctx, texts, opts...)
		if err == nil {
			return vecs, nil
		}
		lastErr = err
		if !isRetryableEmbedErr(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (e *resilientEmbedder) call(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	select {
	case e.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-e.sem }()
	return e.inner.EmbedStrings(ctx, texts, opts...)
}

func waitEmbedRetry(ctx context.Context, attempt int, base time.Duration) error {
	if base <= 0 {
		return nil
	}
	d := base * time.Duration(1<<uint(attempt))
	if d > maxEmbedRetryBackoff {
		d = maxEmbedRetryBackoff
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func isRetryableEmbedErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"):
		return true
	case strings.Contains(msg, "429"):
		return true
	case strings.Contains(msg, "rate limit"):
		return true
	case strings.Contains(msg, "too many requests"):
		return true
	case strings.Contains(msg, "502"):
		return true
	case strings.Contains(msg, "503"):
		return true
	case strings.Contains(msg, "504"):
		return true
	case strings.Contains(msg, "529"):
		return true
	case strings.Contains(msg, "overloaded"):
		return true
	case strings.Contains(msg, "temporarily"):
		return true
	case strings.Contains(msg, "connection reset"):
		return true
	case strings.Contains(msg, "eof"):
		return true
	default:
		return false
	}
}
