// Package task 提供"任务 + 调度"基础能力（DB 版 asynq 语义，见 Spec §5.7）。
//
// 价值：任务入队可与业务写入**同事务**（outbox）；领取用 SKIP LOCKED 支持多实例并发；
// 租约解决 worker 崩溃后任务卡死；定时任务只负责入队，执行路径唯一。
package asyncd

import (
	"math"
	"math/rand"
	"time"

	"polaris/internal/repository"
)

// 默认策略
const (
	// DefaultLeaseTTL 任务租约：worker 领取后按该时长持有，需定期续期
	DefaultLeaseTTL = 60 * time.Second
	// DefaultMaxAttempts 默认最大尝试次数（含首次）
	DefaultMaxAttempts = 8
	// DefaultBackoffBase 退避基数
	DefaultBackoffBase = 2 * time.Second
	// DefaultBackoffCap 退避上限
	DefaultBackoffCap = 10 * time.Minute
	// DefaultPollInterval 领取轮询间隔（多实例下即可满足秒级时效）
	DefaultPollInterval = time.Second
	// DefaultBatchSize 单次领取条数
	DefaultBatchSize = 10
)

// Backoff 计算第 attempts 次失败后的退避时长：指数增长 + 抖动，上限 cap。
// 抖动避免"同一批任务失败后同时重试"造成尖峰（asynq 的 RetryDelayFunc 同思路）。
func Backoff(attempts int, base, cap time.Duration, jitter func() float64) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if base <= 0 {
		base = DefaultBackoffBase
	}
	if cap <= 0 {
		cap = DefaultBackoffCap
	}
	// 指数部分：base * 2^(attempts-1)，先做上限裁剪避免溢出
	exp := float64(base) * math.Pow(2, float64(attempts-1))
	if exp > float64(cap) || math.IsInf(exp, 1) {
		exp = float64(cap)
	}
	// 抖动：±20%
	if jitter == nil {
		jitter = rand.Float64
	}
	factor := 0.8 + 0.4*jitter()
	delay := time.Duration(exp * factor)
	if delay > cap {
		delay = cap
	}
	if delay < time.Second {
		delay = time.Second
	}
	return delay
}

// NextOccurrence 计算调度项的下一次执行时间（interval 或 daily 多时刻）。
//
// daily 语义对齐参考产品：每天在若干时刻执行；`windowMinutes` 表示"到点后前后该窗口内
// 执行一次即可"。这里返回**下一个到点时刻**，窗口与去重由单飞键（window 内已有待执行
// 任务则跳过）保证，因此不会重复入队。
func NextOccurrence(s repository.AsyncdSchedule, now time.Time) time.Time {
	switch s.Kind {
	case repository.ScheduleKindDaily:
		window := s.WindowMinutes
		if window <= 0 {
			window = 30
		}
		// 命中窗口的最近时刻（含已过去但在窗口内的当天时刻）
		if next, ok := nextDailyWithinWindow(s.DailyTimes, now, window); ok {
			return next
		}
		return nextDaily(s.DailyTimes, now, window)
	default: // interval
		seconds := s.IntervalSeconds
		if seconds <= 0 {
			seconds = 60
		}
		return now.Add(time.Duration(seconds) * time.Second)
	}
}

// nextDailyWithinWindow 返回当天仍处于窗口内的时刻（用于"到点后窗口内补跑一次"）
func nextDailyWithinWindow(times []string, now time.Time, windowMinutes int) (time.Time, bool) {
	best := time.Time{}
	for _, at := range parseDailyTimes(times, now) {
		delta := now.Sub(at)
		if delta >= 0 && delta <= time.Duration(windowMinutes)*time.Minute {
			if best.IsZero() || at.After(best) {
				best = at
			}
		}
	}
	return best, !best.IsZero()
}

// nextDaily 返回严格晚于 now 的最近时刻（默认按窗口起点计算）
func nextDaily(times []string, now time.Time, windowMinutes int) time.Time {
	var best time.Time
	for _, at := range parseDailyTimes(times, now) {
		candidate := at
		if !candidate.After(now) {
			// 当天该时刻已过：顺延到明天
			next := candidate.Add(24 * time.Hour)
			if !next.After(now) {
				continue
			}
			candidate = next
		}
		if best.IsZero() || candidate.Before(best) {
			best = candidate
		}
	}
	if best.IsZero() {
		// 未配置时刻：退化为一小时后，避免死循环
		return now.Add(time.Hour)
	}
	_ = windowMinutes
	return best
}

// parseDailyTimes 把 "HH:MM" 解析为当天的时间点（当天日期取自 now）
func parseDailyTimes(times []string, now time.Time) []time.Time {
	out := make([]time.Time, 0, len(times))
	loc := now.Location()
	for _, item := range times {
		parsed, err := time.ParseInLocation("15:04", item, loc)
		if err != nil {
			continue
		}
		out = append(out, time.Date(now.Year(), now.Month(), now.Day(),
			parsed.Hour(), parsed.Minute(), 0, 0, loc))
	}
	return out
}
