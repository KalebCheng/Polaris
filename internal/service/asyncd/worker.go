package asyncd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"polaris/internal/repository"
)

// Worker 任务执行器：多实例安全（领取用 SKIP LOCKED + 租约 + 到期回收）。
type Worker struct {
	service      *Service
	concurrency  int
	leaseTTL     time.Duration
	pollInterval time.Duration
	batchSize    int
	reclaimEvery time.Duration

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// WorkerOption 执行器选项
type WorkerOption func(*Worker)

// WithConcurrency 并发执行数（默认 4）
func WithConcurrency(n int) WorkerOption {
	return func(w *Worker) {
		if n > 0 {
			w.concurrency = n
		}
	}
}

// WithLeaseTTL 租约时长（默认 60s；需大于单任务最长执行时间的 1/3，以保证续期来得及）
func WithLeaseTTL(d time.Duration) WorkerOption {
	return func(w *Worker) {
		if d > 0 {
			w.leaseTTL = d
		}
	}
}

// WithPollInterval 轮询间隔（默认 1s）
func WithPollInterval(d time.Duration) WorkerOption {
	return func(w *Worker) {
		if d > 0 {
			w.pollInterval = d
		}
	}
}

// NewWorker 创建执行器
func NewWorker(service *Service, opts ...WorkerOption) *Worker {
	w := &Worker{
		service: service, concurrency: 4, leaseTTL: DefaultLeaseTTL,
		pollInterval: DefaultPollInterval, batchSize: DefaultBatchSize,
		reclaimEvery: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Start 启动执行器（幂等；由 app 在启动阶段调用）
func (w *Worker) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	for i := 0; i < w.concurrency; i++ {
		workerID := fmt.Sprintf("%s#%d", w.service.workerID, i)
		w.wg.Add(1)
		go func(id string) {
			defer w.wg.Done()
			w.loop(runCtx, id)
		}(workerID)
	}
	// 崩溃恢复：定期把租约过期的任务放回待领取（worker 被 kill 时不会永久卡住）
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.reclaimEvery)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if reclaimed, err := w.service.tasks.ReclaimExpired(runCtx, 50); err != nil {
					logWarn("reclaim expired failed", "error", err)
				} else if reclaimed > 0 {
					logWarn("reclaimed expired tasks", "count", reclaimed)
				}
			}
		}
	}()
	logWarn("worker started", "concurrency", w.concurrency, "lease_ttl", w.leaseTTL.String())
}

// Stop 停止执行器（停止领取，等在途任务收尾）
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

func (w *Worker) loop(ctx context.Context, workerID string) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tasks, err := w.service.tasks.Claim(ctx, workerID, w.leaseTTL, w.batchSize)
			if err != nil {
				logWarn("claim failed", "error", err)
				continue
			}
			for _, task := range tasks {
				w.run(ctx, workerID, task)
			}
		}
	}
}

// run 执行单个任务：续租心跳 → 处理器 → 完成/重试/死信
func (w *Worker) run(ctx context.Context, workerID string, task *repository.AsyncdTask) {
	entry, ok := w.service.registry.Lookup(task.Type)
	if !ok {
		// 类型已下线（或旧数据）：入死信等待人工处理，而不是无限重试
		if err := w.service.tasks.Archive(ctx, task.ID, workerID, errNoHandler(task.Type, w.service.registry.Types()).Error()); err != nil {
			logWarn("archive unknown task failed", "task_id", task.ID, "error", err)
		}
		return
	}

	// 任务级超时：deadline 到期时间与类型默认超时取更紧的一个
	execCtx := ctx
	var cancel context.CancelFunc
	if entry.timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, entry.timeout)
	} else if task.Deadline != nil {
		if remaining := time.Until(*task.Deadline); remaining > 0 {
			execCtx, cancel = context.WithTimeout(ctx, remaining)
		}
	}
	if cancel != nil {
		defer cancel()
	}

	// 心跳续租：租约丢失（被其他实例回收）则停止执行，避免重复写入
	heartbeatCtx, stopHeartbeat := context.WithCancel(execCtx)
	defer stopHeartbeat()
	go w.heartbeat(heartbeatCtx, task.ID, workerID)

	var payload map[string]any
	if len(task.Payload) > 0 {
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			// 载荷损坏：不可重试，直接入死信
			_ = w.service.tasks.Archive(ctx, task.ID, workerID, "invalid payload: "+err.Error())
			return
		}
	}

	started := time.Now()
	err := entry.handler(execCtx, task, payload)
	if err == nil {
		if completeErr := w.service.tasks.Complete(ctx, task.ID, workerID); completeErr != nil {
			logWarn("complete task failed", "task_id", task.ID, "error", completeErr)
		}
		logWarn("task done", "type", task.Type, "task_id", task.ID,
			"attempts", task.Attempts, "duration_ms", time.Since(started).Milliseconds())
		return
	}

	// 失败：退避重试（达到上限自动入死信）
	next := time.Now().Add(Backoff(task.Attempts, DefaultBackoffBase, DefaultBackoffCap, nil))
	if retryErr := w.service.tasks.Retry(ctx, task.ID, workerID, truncate(err.Error(), 500), next); retryErr != nil {
		logWarn("retry task failed", "task_id", task.ID, "error", retryErr)
		return
	}
	logWarn("task failed", "type", task.Type, "task_id", task.ID, "attempts", task.Attempts,
		"error", err, "next_attempt_at", next.Format(time.RFC3339))
}

func (w *Worker) heartbeat(ctx context.Context, taskID int64, workerID string) {
	interval := w.leaseTTL / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			alive, err := w.service.tasks.RenewLease(ctx, taskID, workerID, w.leaseTTL)
			if err != nil {
				logWarn("renew lease failed", "task_id", taskID, "error", err)
				continue
			}
			if !alive {
				// 租约已被回收：本实例不应再继续执行
				logWarn("lease lost, task will be retried elsewhere", "task_id", taskID)
				return
			}
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
