package asyncd

import (
	"context"
	"testing"
	"time"

	"polaris/internal/repository"
)

// 回归：退避必须单调增长、有上限、带抖动且不短于 1s（避免失败任务把 CPU 打满）。
func TestBackoff(t *testing.T) {
	fixed := func(v float64) func() float64 { return func() float64 { return v } }
	prev := time.Duration(0)
	for attempt := 1; attempt <= 12; attempt++ {
		delay := Backoff(attempt, 2*time.Second, 10*time.Minute, fixed(0.5))
		if delay < time.Second {
			t.Fatalf("第 %d 次退避不应短于 1s，实际 %s", attempt, delay)
		}
		if delay > 10*time.Minute {
			t.Fatalf("第 %d 次退避不应超过上限，实际 %s", attempt, delay)
		}
		if attempt > 1 && delay < prev {
			t.Fatalf("退避应单调不减：attempt=%d delay=%s prev=%s", attempt, delay, prev)
		}
		prev = delay
	}
	// 抖动区间：同一 attempts 下 0 与 1 的结果都应落在 ±20% 内且不同
	low := Backoff(6, time.Second, time.Hour, fixed(0))
	high := Backoff(6, time.Second, time.Hour, fixed(1))
	if !(high > low) {
		t.Fatalf("抖动应产生差异：low=%s high=%s", low, high)
	}
	if high > time.Duration(float64(low)*2) {
		t.Fatalf("抖动幅度过大：low=%s high=%s", low, high)
	}
}

// 回归：interval 调度按间隔推进；daily 调度取下一个时刻并顺延到次日。
func TestNextOccurrence(t *testing.T) {
	now := time.Date(2026, 10, 13, 4, 0, 0, 0, time.UTC)

	interval := NextOccurrence(repository.AsyncdSchedule{Kind: repository.ScheduleKindInterval, IntervalSeconds: 3600}, now)
	if got := interval.Sub(now); got != time.Hour {
		t.Fatalf("interval 调度应为 1 小时后，实际 %s", got)
	}

	daily := NextOccurrence(repository.AsyncdSchedule{
		Kind: repository.ScheduleKindDaily, DailyTimes: []string{"03:00", "05:00"}, WindowMinutes: 30,
	}, now)
	if daily.Hour() != 5 || daily.Day() != 13 {
		t.Fatalf("4:00 应命中当天 5:00，实际 %s", daily)
	}

	// 4:10 距 03:00 已超过 30 分钟窗口 → 下一个时刻为当天 05:00
	late := NextOccurrence(repository.AsyncdSchedule{
		Kind: repository.ScheduleKindDaily, DailyTimes: []string{"03:00", "05:00"}, WindowMinutes: 30,
	}, time.Date(2026, 10, 13, 4, 10, 0, 0, time.UTC))
	if late.Hour() != 5 {
		t.Fatalf("超出窗口应取下一个时刻，实际 %s", late)
	}

	// 4:05 仍在 03:00 的窗口内 → 命中当天 03:00（窗口内补跑一次）
	inWindow := NextOccurrence(repository.AsyncdSchedule{
		Kind: repository.ScheduleKindDaily, DailyTimes: []string{"03:00", "05:00"}, WindowMinutes: 30,
	}, time.Date(2026, 10, 13, 3, 20, 0, 0, time.UTC))
	if inWindow.Hour() != 3 {
		t.Fatalf("窗口内应命中当天 03:00，实际 %s", inWindow)
	}

	// 当天时刻都已过 → 顺延到次日第一个时刻
	tomorrow := NextOccurrence(repository.AsyncdSchedule{
		Kind: repository.ScheduleKindDaily, DailyTimes: []string{"03:00", "05:00"}, WindowMinutes: 30,
	}, time.Date(2026, 10, 13, 23, 0, 0, 0, time.UTC))
	if tomorrow.Day() != 14 || tomorrow.Hour() != 3 {
		t.Fatalf("应顺延到次日 03:00，实际 %s", tomorrow)
	}
}

// 回归：未注册的任务类型拒绝入队（避免"入了队没人执行"的静默堆积）。
func TestRegistryRejectsUnknownType(t *testing.T) {
	registry := NewRegistry()
	if _, ok := registry.Lookup("unknown.task"); ok {
		t.Fatal("未注册的类型不应被查到")
	}
	registry.Register("demo.task", func(ctx context.Context, task *repository.AsyncdTask, payload map[string]any) error {
		return nil
	})
	if _, ok := registry.Lookup("demo.task"); !ok {
		t.Fatal("注册后应可查到")
	}
	if len(registry.Types()) != 1 || registry.Types()[0] != "demo.task" {
		t.Fatalf("类型清单错误: %v", registry.Types())
	}
}
