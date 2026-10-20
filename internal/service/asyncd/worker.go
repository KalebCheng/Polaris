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
//
// 每个并发槽位都是一个**独立登记的执行器实例**（asyncd_workers 一行，id = <host>-<pid>#<slot>）：
// 领取时写进 asyncd_job_runs.worker_id / asyncd_job_run_attempts.worker_id，心跳 10s、失联阈值 30s，
// 于是"有几个服务在跑、各自几个槽位、谁领了这个任务"都能在界面上直接回答。
type Worker struct {
	service      *Service
	concurrency  int
	leaseTTL     time.Duration
	pollInterval time.Duration
	batchSize    int
	reclaimEvery time.Duration
	heartbeat    time.Duration

	// slots 已登记的槽位 id（停止时逐个标记 stopped_at）
	slots  []string
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// WorkerOption 执行器选项
type WorkerOption func(*Worker)

// WithConcurrency 并发槽位数（默认 4）
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

// WithPollInterval 领取轮询间隔（默认 1s）
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
		reclaimEvery: ReclaimEvery, heartbeat: WorkerHeartbeatInterval,
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
		record := w.service.workerRecord(i)
		// 登记失败不阻断启动：执行器照常领取任务，只是这一轮界面上看不到它
		if err := w.service.workers.Register(runCtx, record); err != nil {
			logWarn("register worker failed", "worker", record.ID, "error", err)
		}
		w.slots = append(w.slots, record.ID)
		w.wg.Add(1)
		go func(id string) {
			defer w.wg.Done()
			w.loop(runCtx, id)
		}(record.ID)
	}

	// 维护循环：①执行器心跳 ②租约过期回收（崩溃恢复）
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		heartbeat := time.NewTicker(w.heartbeat)
		defer heartbeat.Stop()
		reclaim := time.NewTicker(w.reclaimEvery)
		defer reclaim.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-heartbeat.C:
				for _, slot := range w.slots {
					if err := w.service.workers.Heartbeat(runCtx, slot); err != nil {
						logWarn("worker heartbeat failed", "worker", slot, "error", err)
					}
				}
			case <-reclaim.C:
				reclaimed, err := w.service.jobRuns.ReclaimExpired(runCtx, 50)
				if err != nil {
					logWarn("reclaim expired runs failed", "error", err)
				} else if len(reclaimed) > 0 {
					logWarn("reclaimed expired job runs", "count", len(reclaimed))
				}
			}
		}
	}()

	logWarn("worker started", "instance", w.service.InstanceID(), "concurrency", w.concurrency,
		"lease_ttl", w.leaseTTL.String(), "heartbeat", w.heartbeat.String())
}

// Stop 停止执行器（停止领取，等在途任务收尾，并把槽位标记为已退出）
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	// 退出标记必须用新上下文：runCtx 已经取消
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, slot := range w.slots {
		if err := w.service.workers.Stop(ctx, slot); err != nil {
			logWarn("mark worker stopped failed", "worker", slot, "error", err)
		}
	}
}

func (w *Worker) loop(ctx context.Context, workerID string) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			claimed, err := w.service.jobRuns.Claim(ctx, workerID, w.leaseTTL, w.batchSize)
			if err != nil {
				logWarn("claim failed", "error", err)
				continue
			}
			for _, jobRun := range claimed {
				w.execute(ctx, workerID, jobRun)
			}
		}
	}
}

// execute 执行单个实例：开尝试记录 → 续租心跳 → 处理器 → 完成/重试/死信 → 关尝试记录
func (w *Worker) execute(ctx context.Context, workerID string, jobRun *repository.AsyncdJobRun) {
	entry, ok := w.service.registry.Lookup(jobRun.TaskType)
	if !ok {
		// 定义指向的任务类型已下线（或旧数据）：入死信等待人工处理，而不是无限重试
		w.finishJobRun(ctx, workerID, jobRun, nil, errNoHandler(jobRun.TaskType, w.service.registry.Types()), false)
		return
	}

	// 执行记录先落一行 running：进程被杀时这行会被租约回收流程标成 lease_lost，
	// 于是"这次尝试到底跑没跑、跑了多久"有据可查（旧模型里这种尝试不留任何痕迹）。
	attemptID, err := w.service.runAttempts.Start(ctx, &repository.AsyncdJobRunAttempt{
		JobRunID: jobRun.ID, AttemptNo: jobRun.Attempts, WorkerID: workerID,
	})
	if err != nil {
		// 执行记录写不进去仍然继续执行：记录是观测手段，不该阻断业务
		logWarn("start job run attempt failed", "job_run_id", jobRun.ID, "error", err)
	}

	// 任务级超时：deadline 到期时间与类型默认超时取更紧的一个
	execCtx := ctx
	var cancel context.CancelFunc
	if entry.timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, entry.timeout)
	} else if jobRun.Deadline != nil {
		if remaining := time.Until(*jobRun.Deadline); remaining > 0 {
			execCtx, cancel = context.WithTimeout(ctx, remaining)
		}
	}
	if cancel != nil {
		defer cancel()
	}

	// 心跳续租：租约丢失（被其他实例回收）则停止执行，避免重复写入
	heartbeatCtx, stopHeartbeat := context.WithCancel(execCtx)
	defer stopHeartbeat()
	go w.heartbeatLoop(heartbeatCtx, jobRun.ID, workerID)

	var payload map[string]any
	if len(jobRun.Payload) > 0 {
		if err := json.Unmarshal(jobRun.Payload, &payload); err != nil {
			w.finishJobRun(ctx, workerID, jobRun, &attemptID, fmt.Errorf("invalid payload: %w", err), false)
			return
		}
	}

	started := time.Now()
	handlerErr := entry.handler(execCtx, jobRun, payload)
	// 处理器失败按退避重试（超 max_attempts 由仓库侧转死信）
	w.finishJobRun(ctx, workerID, jobRun, &attemptID, handlerErr, true)
	logWarn("job run finished", "type", jobRun.TaskType, "job_run_id", jobRun.ID, "attempts", jobRun.Attempts,
		"duration_ms", time.Since(started).Milliseconds(), "error", handlerErr)
}

// finishJobRun 结算一次执行：写执行记录（成功/失败）+ 更新执行实例（成功/重试/死信）。
//
// retryable=false 用于"重试也不会好"的失败（载荷损坏、任务类型已下线）——直接进死信，
// 避免把明确不可恢复的错误反复重试到耗尽次数。
func (w *Worker) finishJobRun(
	ctx context.Context, workerID string, jobRun *repository.AsyncdJobRun, attemptID *int64, cause error, retryable bool,
) {
	closeAttempt := func(status, message string) {
		if attemptID == nil {
			return
		}
		if err := w.service.runAttempts.Finish(ctx, *attemptID, status, message); err != nil {
			logWarn("finish attempt failed", "job_run_id", jobRun.ID, "error", err)
		}
	}

	if cause == nil {
		closeAttempt(repository.AsyncdJobRunAttemptSucceeded, "")
		if err := w.service.jobRuns.Complete(ctx, jobRun.ID, workerID); err != nil {
			logWarn("complete job run failed", "job_run_id", jobRun.ID, "error", err)
		}
		return
	}

	message := truncate(cause.Error(), 500)
	closeAttempt(repository.AsyncdJobRunAttemptFailed, message)
	if !retryable {
		if err := w.service.jobRuns.Kill(ctx, jobRun.ID, workerID, message); err != nil {
			logWarn("kill job run failed", "job_run_id", jobRun.ID, "error", err)
		}
		return
	}
	next := time.Now().Add(Backoff(jobRun.Attempts, DefaultBackoffBase, DefaultBackoffCap, nil))
	if err := w.service.jobRuns.Retry(ctx, jobRun.ID, workerID, message, next); err != nil {
		logWarn("retry job run failed", "job_run_id", jobRun.ID, "error", err)
	}
}

func (w *Worker) heartbeatLoop(ctx context.Context, jobRunID int64, workerID string) {
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
			alive, err := w.service.jobRuns.RenewLease(ctx, jobRunID, workerID, w.leaseTTL)
			if err != nil {
				logWarn("renew lease failed", "job_run_id", jobRunID, "error", err)
				continue
			}
			if !alive {
				// 租约已被回收：本实例不应再继续执行
				logWarn("lease lost, job run will be retried elsewhere", "job_run_id", jobRunID)
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
