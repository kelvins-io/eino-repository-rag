package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
	dbmodel "github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

// IndexJob Redis 索引队列任务
type IndexJob struct {
	DocID   uint   `json:"doc_id"`
	Attempt int    `json:"attempt"`
	Error   string `json:"error,omitempty"`
}

// IndexQueue Redis List 持久化索引队列 + worker 池
type IndexQueue struct {
	rdb     *redis.Client
	cfg     config.RAGConfig
	pipe    *Pipeline
	docRepo *repository.DocumentRepo

	queueKey  string
	activeKey string
	dlqKey    string
	dedupKey  string

	cancel  context.CancelFunc
	runCtx  context.Context
	wg      sync.WaitGroup
	mu      sync.Mutex
	started bool
}

func NewIndexQueue(cfg *config.Config, rdb *redis.Client, pipe *Pipeline, docRepo *repository.DocumentRepo) *IndexQueue {
	return &IndexQueue{
		rdb:       rdb,
		cfg:       cfg.RAG,
		pipe:      pipe,
		docRepo:   docRepo,
		queueKey:  cfg.RAG.IndexQueueKey,
		activeKey: cfg.RAG.IndexActiveKey,
		dlqKey:    cfg.RAG.IndexDLQKey,
		dedupKey:  cfg.RAG.IndexDedupKey,
	}
}

// Enqueue 将文档加入索引队列；已在排队/执行中则跳过
func (q *IndexQueue) Enqueue(ctx context.Context, docID uint) error {
	if q == nil || docID == 0 {
		return nil
	}
	job := IndexJob{DocID: docID, Attempt: 0}
	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal index job: %w", err)
	}
	member := strconv.FormatUint(uint64(docID), 10)

	// 原子：仅首次入去重集时 LPUSH，避免重复排队
	script := redis.NewScript(`
if redis.call('SADD', KEYS[1], ARGV[1]) == 0 then
  return 0
end
redis.call('LPUSH', KEYS[2], ARGV[2])
return 1
`)
	n, err := script.Run(ctx, q.rdb, []string{q.dedupKey, q.queueKey}, member, string(raw)).Int()
	if err != nil {
		return fmt.Errorf("enqueue index job doc_id=%d: %w", docID, err)
	}
	if n == 0 {
		log.Printf("[rag] index queue skip duplicate doc_id=%d", docID)
		return nil
	}
	log.Printf("[rag] index queue enqueued doc_id=%d", docID)
	return nil
}

// Start 回灌在途任务、回收 DB pending/indexing，并启动 worker
func (q *IndexQueue) Start(ctx context.Context) error {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.started {
		return nil
	}

	if err := q.reclaimActive(ctx); err != nil {
		return fmt.Errorf("reclaim active index jobs: %w", err)
	}
	if err := q.rebuildDedup(ctx); err != nil {
		return fmt.Errorf("rebuild index dedup: %w", err)
	}
	if err := q.reclaimFromDB(ctx); err != nil {
		return fmt.Errorf("reclaim db index jobs: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	q.cancel = cancel
	q.runCtx = runCtx
	workers := q.cfg.IndexWorkers
	if workers <= 0 {
		workers = 2
	}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(runCtx, i)
	}
	q.started = true
	log.Printf("[rag] index queue started workers=%d queue=%s", workers, q.queueKey)
	return nil
}

// Stop 停止 worker 并等待退出；在途任务留在 active，下次 Start 回灌
func (q *IndexQueue) Stop() {
	if q == nil {
		return
	}
	q.mu.Lock()
	if !q.started {
		q.mu.Unlock()
		return
	}
	cancel := q.cancel
	q.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	q.wg.Wait()

	q.mu.Lock()
	q.started = false
	q.cancel = nil
	q.runCtx = nil
	q.mu.Unlock()
	log.Printf("[rag] index queue stopped")
}

func (q *IndexQueue) reclaimActive(ctx context.Context) error {
	moved := 0
	for {
		raw, err := q.rdb.RPopLPush(ctx, q.activeKey, q.queueKey).Result()
		if errors.Is(err, redis.Nil) {
			break
		}
		if err != nil {
			return err
		}
		_ = raw
		moved++
	}
	if moved > 0 {
		log.Printf("[rag] index queue reclaimed %d active job(s) to queue", moved)
	}
	return nil
}

func (q *IndexQueue) rebuildDedup(ctx context.Context) error {
	if err := q.rdb.Del(ctx, q.dedupKey).Err(); err != nil {
		return err
	}
	for _, key := range []string{q.queueKey, q.activeKey} {
		items, err := q.rdb.LRange(ctx, key, 0, -1).Result()
		if err != nil {
			return err
		}
		for _, raw := range items {
			job, err := parseIndexJob(raw)
			if err != nil || job.DocID == 0 {
				continue
			}
			if err := q.rdb.SAdd(ctx, q.dedupKey, strconv.FormatUint(uint64(job.DocID), 10)).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (q *IndexQueue) reclaimFromDB(ctx context.Context) error {
	ids, err := q.docRepo.ListIDsByStatuses(dbmodel.DocumentStatusPending, dbmodel.DocumentStatusIndexing)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.Enqueue(ctx, id); err != nil {
			log.Printf("[rag] index queue reclaim enqueue failed doc_id=%d err=%v", id, err)
		}
	}
	if len(ids) > 0 {
		log.Printf("[rag] index queue db reclaim candidates=%d", len(ids))
	}
	return nil
}

func (q *IndexQueue) worker(ctx context.Context, id int) {
	defer q.wg.Done()
	log.Printf("[rag] index worker-%d started", id)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[rag] index worker-%d stopping", id)
			return
		default:
		}

		raw, err := q.rdb.BLMove(ctx, q.queueKey, q.activeKey, "RIGHT", "LEFT", 2*time.Second).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Printf("[rag] index worker-%d stopping", id)
				return
			}
			// go-redis 在 ctx cancel 时可能返回包装错误
			if ctx.Err() != nil {
				log.Printf("[rag] index worker-%d stopping", id)
				return
			}
			log.Printf("[rag] index worker-%d blmove err=%v", id, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		q.handleJob(raw)
	}
}

func (q *IndexQueue) handleJob(raw string) {
	job, err := parseIndexJob(raw)
	if err != nil {
		log.Printf("[rag] index queue invalid job payload=%q err=%v", truncate(raw, 200), err)
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = q.rdb.LRem(cleanupCtx, q.activeKey, 1, raw).Err()
		cancel()
		return
	}

	timeout := time.Duration(q.cfg.IndexJobTimeoutMinutes) * time.Minute
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := q.process(ctx, job); err != nil {
		q.onFailure(raw, job, err)
		return
	}
	q.onSuccess(raw, job)
}

func (q *IndexQueue) process(ctx context.Context, job IndexJob) error {
	_, err := q.docRepo.GetByID(job.DocID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[rag] index queue skip missing doc_id=%d", job.DocID)
			return errDocGone
		}
		return err
	}
	return q.pipe.IndexDocument(ctx, job.DocID)
}

var errDocGone = errors.New("document gone")

func (q *IndexQueue) redisOpCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func (q *IndexQueue) onSuccess(raw string, job IndexJob) {
	ctx, cancel := q.redisOpCtx()
	defer cancel()
	_ = q.rdb.LRem(ctx, q.activeKey, 1, raw).Err()
	_ = q.rdb.SRem(ctx, q.dedupKey, strconv.FormatUint(uint64(job.DocID), 10)).Err()
	log.Printf("[rag] index queue done doc_id=%d attempt=%d", job.DocID, job.Attempt)
}

func (q *IndexQueue) onFailure(raw string, job IndexJob, err error) {
	ctx, cancel := q.redisOpCtx()
	_ = q.rdb.LRem(ctx, q.activeKey, 1, raw).Err()
	member := strconv.FormatUint(uint64(job.DocID), 10)

	if errors.Is(err, errDocGone) {
		_ = q.rdb.SRem(ctx, q.dedupKey, member).Err()
		cancel()
		return
	}

	maxRetries := q.cfg.IndexMaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	if job.Attempt < maxRetries {
		_ = q.docRepo.UpdateStatus(job.DocID, dbmodel.DocumentStatusPending, 0, err.Error())
		retry := IndexJob{DocID: job.DocID, Attempt: job.Attempt + 1, Error: err.Error()}
		payload, merr := json.Marshal(retry)
		if merr != nil {
			log.Printf("[rag] index queue marshal retry failed doc_id=%d err=%v", job.DocID, merr)
			_ = q.rdb.SRem(ctx, q.dedupKey, member).Err()
			cancel()
			return
		}
		cancel()
		backoff := indexRetryBackoff(retry.Attempt, q.cfg.IndexRetryBackoffSeconds)
		log.Printf("[rag] index queue retry doc_id=%d attempt=%d/%d backoff=%s err=%v",
			job.DocID, retry.Attempt, maxRetries, backoff, err)
		q.waitRetryBackoff(backoff)
		pushCtx, pushCancel := q.redisOpCtx()
		defer pushCancel()
		if perr := q.rdb.LPush(pushCtx, q.queueKey, string(payload)).Err(); perr != nil {
			log.Printf("[rag] index queue requeue failed doc_id=%d err=%v", job.DocID, perr)
			_ = q.rdb.SRem(pushCtx, q.dedupKey, member).Err()
			return
		}
		return
	}

	// 最终失败：IndexDocument 已写 failed；进死信并释放去重
	dlqJob := IndexJob{DocID: job.DocID, Attempt: job.Attempt, Error: err.Error()}
	if payload, merr := json.Marshal(dlqJob); merr == nil {
		_ = q.rdb.LPush(ctx, q.dlqKey, string(payload)).Err()
	}
	_ = q.rdb.SRem(ctx, q.dedupKey, member).Err()
	cancel()
	log.Printf("[rag] index queue dead-letter doc_id=%d attempt=%d err=%v", job.DocID, job.Attempt, err)
}

func (q *IndexQueue) waitRetryBackoff(d time.Duration) {
	if d <= 0 {
		return
	}
	q.mu.Lock()
	runCtx := q.runCtx
	q.mu.Unlock()
	if runCtx == nil {
		time.Sleep(d)
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-runCtx.Done():
	case <-t.C:
	}
}

func indexRetryBackoff(attempt, baseSeconds int) time.Duration {
	if baseSeconds <= 0 {
		baseSeconds = 5
	}
	shift := attempt - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 5 {
		shift = 5
	}
	return time.Duration(baseSeconds<<shift) * time.Second
}

func parseIndexJob(raw string) (IndexJob, error) {
	var job IndexJob
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		return IndexJob{}, err
	}
	if job.DocID == 0 {
		return IndexJob{}, fmt.Errorf("missing doc_id")
	}
	return job, nil
}
