package service

import (
	"time"

	"polaris/internal/repository"
	"polaris/internal/service/cron"
)

// DueForSync 判断某个数据源此刻是否该触发一次同步（供 asyncd 的定时扫描任务调用）。
//
// 规则（syncers.sync_config.schedule，与「同步配置」界面一一对应）：
//   - mode=daily：当前时刻落在某个配置时刻的**执行窗口**内，且该窗口内尚未执行过
//     （last_run_time < 窗口起点）→ 触发；这正是"到点前后 30 分钟内执行一次"的语义；
//   - mode=interval：距上次执行已超过间隔 → 触发；
//   - 未配置 schedule：退回历史的 `schedule` 字段（间隔秒数），保持向后兼容。
//
// 说明：真正的"防并发重复跑"由 asyncd 的单飞键（syncer:<id>）保证，这里只判断"该不该跑"。
func DueForSync(sy *repository.Syncer, now time.Time) bool {
	if sy == nil || !sy.IsEnabled {
		return false
	}
	schedule := sy.SyncConfig.Schedule
	switch schedule.Mode {
	case "daily":
		window := schedule.WindowMinutes
		if window <= 0 {
			window = 30
		}
		windowStart := windowStartForDaily(schedule.Times, now, window)
		if windowStart.IsZero() {
			return false
		}
		if sy.LastRunTime == nil {
			return true
		}
		// 窗口内已经跑过（含正在跑）就不再触发
		return sy.LastRunTime.Before(windowStart)
	case "cron":
		parsed, err := cron.Parse(schedule.Cron)
		if err != nil {
			return false // 非法表达式不触发（保存时已被校验拦截）
		}
		from := now
		if sy.LastRunTime != nil {
			from = *sy.LastRunTime
		}
		next, ok := parsed.Next(from)
		if !ok {
			return false
		}
		return !next.After(now)
	case "interval":
		seconds := schedule.IntervalSeconds
		if seconds <= 0 {
			seconds = int(parseScheduleSeconds(sy.Schedule) / time.Second)
		}
		if seconds <= 0 {
			return false
		}
		if sy.LastRunTime == nil {
			return true
		}
		return now.Sub(*sy.LastRunTime) >= time.Duration(seconds)*time.Second
	default:
		interval := parseScheduleSeconds(sy.Schedule)
		if interval <= 0 {
			return false
		}
		if sy.LastRunTime == nil {
			return true
		}
		return now.Sub(*sy.LastRunTime) >= interval
	}
}

// windowStartForDaily 返回当前时刻命中的那个"时刻"（即执行窗口的起点）；未命中窗口返回零值。
func windowStartForDaily(times []string, now time.Time, windowMinutes int) time.Time {
	loc := now.Location()
	window := time.Duration(windowMinutes) * time.Minute
	var hit time.Time
	for _, item := range times {
		parsed, err := time.ParseInLocation("15:04", item, loc)
		if err != nil {
			continue
		}
		at := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)
		if now.Before(at) {
			// 窗口可以提前到"到点前"：把窗口起点投影到前一天同一时刻比较
			at = at.Add(-24 * time.Hour)
		}
		if now.Sub(at) <= window && (hit.IsZero() || at.After(hit)) {
			hit = at
		}
	}
	return hit
}

func atoiOrZero(s string) int {
	value := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		value = value*10 + int(r-'0')
		if value > 1<<30 {
			return 0
		}
	}
	return value
}

// parseScheduleSeconds 解析历史的"间隔秒数"字段（仅数字）
func parseScheduleSeconds(schedule string) time.Duration {
	if schedule == "" {
		return 0
	}
	seconds := atoiOrZero(schedule)
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
