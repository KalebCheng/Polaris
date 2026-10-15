package service

import (
	"testing"
	"time"

	"polaris/internal/repository"
)

// 回归：daily 时刻 + 执行窗口 —— 窗口内只触发一次；窗口外不触发。
func TestDueForSyncDailyWindow(t *testing.T) {
	base := time.Date(2026, 10, 13, 3, 5, 0, 0, time.UTC) // 03:05，落在 03:00 的 30 分钟窗口内
	sy := &repository.Syncer{
		IsEnabled: true,
		SyncConfig: repository.SyncConfig{
			Schedule: repository.SyncSchedule{Mode: "daily", Times: []string{"03:00", "05:00"}, WindowMinutes: 30},
		},
	}
	if !DueForSync(sy, base) {
		t.Fatal("窗口内且从未执行过 → 应触发")
	}

	// 本窗口已执行（上次 03:02）→ 不再重复触发
	ran := time.Date(2026, 10, 13, 3, 2, 0, 0, time.UTC)
	sy.LastRunTime = &ran
	if DueForSync(sy, base) {
		t.Fatal("同一窗口内已执行过 → 不应重复触发")
	}

	// 上次执行发生在窗口之前（昨天 03:00）→ 应触发
	previous := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	sy.LastRunTime = &previous
	if !DueForSync(sy, base) {
		t.Fatal("新窗口尚未执行 → 应触发")
	}

	// 窗口之外（03:45，距 03:00 已 45 分钟）→ 不触发
	outside := time.Date(2026, 10, 13, 3, 45, 0, 0, time.UTC)
	sy.LastRunTime = &previous
	if DueForSync(sy, outside) {
		t.Fatal("窗口之外 → 不应触发")
	}
}

// 回归：interval 模式按间隔触发；未配置 schedule 时退回历史"间隔秒数"字段。
func TestDueForSyncIntervalAndLegacy(t *testing.T) {
	now := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	recent := now.Add(-10 * time.Minute)
	old := now.Add(-2 * time.Hour)

	interval := &repository.Syncer{
		IsEnabled:   true,
		LastRunTime: &recent,
		SyncConfig: repository.SyncConfig{
			Schedule: repository.SyncSchedule{Mode: "interval", IntervalSeconds: 3600},
		},
	}
	if DueForSync(interval, now) {
		t.Fatal("10 分钟 < 1 小时 → 不应触发")
	}
	interval.LastRunTime = &old
	if !DueForSync(interval, now) {
		t.Fatal("2 小时 > 1 小时 → 应触发")
	}

	legacy := &repository.Syncer{IsEnabled: true, Schedule: "1800", LastRunTime: &recent}
	if DueForSync(legacy, now) {
		t.Fatal("历史间隔字段：10 分钟 < 30 分钟 → 不应触发")
	}
	legacy.LastRunTime = &old
	if !DueForSync(legacy, now) {
		t.Fatal("历史间隔字段：超过间隔 → 应触发")
	}

	// 停用 / 未配置调度 → 一律不触发
	if DueForSync(&repository.Syncer{IsEnabled: false, Schedule: "60"}, now) {
		t.Fatal("停用的数据源不应触发")
	}
	if DueForSync(&repository.Syncer{IsEnabled: true}, now) {
		t.Fatal("未配置任何调度 → 不应触发")
	}
}

// 回归：cron 模式的"该不该跑"判定——按上次执行之后的下一次触发时间比较；
// 非法表达式一律不触发（保存时已被服务端校验拦截）。
func TestDueForSyncCron(t *testing.T) {
	now := time.Date(2026, 10, 13, 3, 30, 0, 0, time.UTC)
	sy := &repository.Syncer{
		IsEnabled: true,
		SyncConfig: repository.SyncConfig{
			Schedule: repository.SyncSchedule{Mode: "cron", Cron: "0 3 * * *"},
		},
	}
	// 从未执行过 → 当天 03:00 已过、下一次是次日 → 不触发
	if DueForSync(sy, now) {
		t.Fatal("cron：下一次触发在次日 03:00，当前不应触发")
	}
	// 上次执行远早于今天 → 今天的 03:00 已到 → 触发
	previous := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	sy.LastRunTime = &previous
	if !DueForSync(sy, now) {
		t.Fatal("cron：上次执行在昨天，今天 03:00 已过 → 应触发")
	}
	// 今天 03:00 已执行过 → 不再触发
	ran := time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC)
	sy.LastRunTime = &ran
	if DueForSync(sy, now) {
		t.Fatal("cron：本周期已执行 → 不应重复触发")
	}
	// 非法表达式：不触发（而不是 panic 或误触发）
	sy.SyncConfig.Schedule.Cron = "not a cron"
	sy.LastRunTime = nil
	if DueForSync(sy, now) {
		t.Fatal("cron：非法表达式不应触发")
	}
}
