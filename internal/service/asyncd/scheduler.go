package asyncd

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Scheduler 定时调度：**只负责入队**（执行统一走 Worker，Spec §5.7）。
//
// 多实例安全：ClaimDue 用一条 UPDATE ... RETURNING + FOR UPDATE SKIP LOCKED 抢占，
// 同一时刻只有一个实例能拿到某个调度项，因此同一周期只会入队一次。
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

// tick 抢占到点的调度项并入队
func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now()
	due, err := s.service.schedules.ClaimDue(ctx, now, 20)
	if err != nil {
		logWarn("claim schedules failed", "error", err)
		return
	}
	for _, schedule := range due {
		next := NextOccurrence(*schedule, now)
		status := "enqueued"
		// 单飞键按调度项唯一：窗口内已有待执行/执行中的任务时不会重复入队（对应"窗口内只跑一次"）
		if _, existing, enqueueErr := s.service.Enqueue(ctx, schedule.TaskType, decodePayload(schedule.Payload),
			"schedule:"+schedule.JobName, now, WithMaxAttempts(DefaultMaxAttempts)); enqueueErr != nil {
			status = "failed"
			logWarn("enqueue scheduled task failed", "job", schedule.JobName, "error", enqueueErr)
		} else if existing {
			logWarn("scheduled task already queued, skipped", "job", schedule.JobName)
		}
		if markErr := s.service.schedules.MarkRun(ctx, schedule.ID, status, next, now); markErr != nil {
			logWarn("mark schedule run failed", "job", schedule.JobName, "error", markErr)
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
