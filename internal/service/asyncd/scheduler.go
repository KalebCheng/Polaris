package asyncd

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"polaris/internal/repository"
)

// Scheduler 定时任务调度器：扫描到点的任务定义，**只负责入队**，执行统一走 worker。
//
// 多实例安全：ClaimDue 用一条 UPDATE ... RETURNING + FOR UPDATE SKIP LOCKED 抢占，
// 同一时刻只有一个实例能拿到某个任务定义，因此同一周期只会入队一次。
type Scheduler struct {
	service  *Service
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// SchedulerOption 调度器选项
type SchedulerOption func(*Scheduler)

// WithScanInterval 扫描间隔（默认 1s）
func WithScanInterval(d time.Duration) SchedulerOption {
	return func(s *Scheduler) {
		if d > 0 {
			s.interval = d
		}
	}
}

// NewScheduler 创建调度器
func NewScheduler(service *Service, opts ...SchedulerOption) *Scheduler {
	s := &Scheduler{service: service, interval: time.Second}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start 启动扫描
func (s *Scheduler) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				s.tick(runCtx)
			}
		}
	}()
	logWarn("scheduler started", "scan_interval", s.interval.String())
}

// Stop 停止扫描
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// tick 抢占到点的任务定义并入队
func (s *Scheduler) tick(ctx context.Context) {
	now := s.service.clock()
	due, err := s.service.jobs.ClaimDue(ctx, now, 20)
	if err != nil {
		logWarn("claim due jobs failed", "error", err)
		return
	}
	for _, job := range due {
		next, nextErr := NextOccurrence(*job, now)
		status := repository.AsyncdJobStatusQueued
		switch {
		case nextErr != nil:
			// 规则非法（历史脏数据/手改的行）：不调度，如实记 failed，并把下次时间推到 1 小时后避免热循环
			status = repository.AsyncdJobStatusFailed
			next = now.Add(time.Hour)
			logWarn("next occurrence failed", "job", job.Name, "error", nextErr)
		default:
			// 单飞键来自定义：代码声明的任务回落 job:<name>；业务持有的任务由持有方自定（如 <资源>:<id>），
			// 于是"手工点立即同步"与"定时到点"共用同一个键，不会并发跑同一连接。
			uniqueKey := job.UniqueKey
			if uniqueKey == "" {
				uniqueKey = "job:" + job.Name
			}
			if _, existing, enqueueErr := s.service.Enqueue(ctx, job.Name, decodePayload(job.Payload),
				uniqueKey, now, WithMaxAttempts(DefaultMaxAttempts)); enqueueErr != nil {
				status = repository.AsyncdJobStatusFailed
				logWarn("enqueue scheduled job failed", "job", job.Name, "error", enqueueErr)
			} else if existing {
				status = repository.AsyncdJobStatusSkipped
				logWarn("scheduled job already queued, skipped", "job", job.Name)
			}
		}
		if markErr := s.service.jobs.MarkRun(ctx, job.ID, status, next, now); markErr != nil {
			logWarn("mark job run failed", "job", job.Name, "error", markErr)
		}
	}
}

func decodePayload(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
