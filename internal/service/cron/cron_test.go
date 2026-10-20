package cron

import (
	"testing"
	"time"
)

// 回归：常见表达式的下一次触发时间必须正确（含"每天固定时刻""每 N 分钟""工作日"）。
func TestNext(t *testing.T) {
	base := time.Date(2026, 10, 13, 10, 30, 0, 0, time.UTC) // 周二
	cases := []struct {
		expr string
		want string
	}{
		{"0 3 * * *", "2026-10-14 03:00"},     // 每天 03:00（当天已过 → 次日）
		{"*/15 * * * *", "2026-10-13 10:45"},  // 每 15 分钟
		{"0 5 * * 1", "2026-10-19 05:00"},     // 每周一 05:00
		{"30 10 13 10 *", "2027-10-13 10:30"}, // 每年 10 月 13 日 10:30（当天该时刻已过）
		{"0 0 1 * *", "2026-11-01 00:00"},     // 每月 1 日
		{"0 3,5 * * *", "2026-10-14 03:00"},   // 每天 03:00 与 05:00
		{"0 9-17 * * *", "2026-10-13 11:00"},  // 工作时段：10:30 的下一次是当天 11:00
		{"59 23 31 12 *", "2026-12-31 23:59"}, // 年末
	}
	for _, tc := range cases {
		schedule, err := Parse(tc.expr)
		if err != nil {
			t.Fatalf("解析 %q 失败: %v", tc.expr, err)
		}
		got, ok := schedule.Next(base)
		if !ok {
			t.Fatalf("%q 未算出下一次触发", tc.expr)
		}
		if got.Format("2006-01-02 15:04") != tc.want {
			t.Errorf("%q 下一次触发 = %s，期望 %s", tc.expr, got.Format("2006-01-02 15:04"), tc.want)
		}
	}
}

// 回归：非法表达式必须被拒绝（段数、越界、非法字符），且错误信息可读。
func TestParseRejectsInvalid(t *testing.T) {
	for _, expr := range []string{
		"", "* * * *", "60 * * * *", "* 24 * * *", "* * 32 * *", "* * * 13 *",
		"* * * * 8", "abc * * * *", "*/0 * * * *", "5-1 * * * *",
	} {
		if _, err := Parse(expr); err == nil {
			t.Errorf("表达式 %q 应被拒绝", expr)
		}
	}
}

// 回归：周日既可写 0 也可写 7
func TestSundayAlias(t *testing.T) {
	for _, expr := range []string{"0 3 * * 0", "0 3 * * 7"} {
		schedule, err := Parse(expr)
		if err != nil {
			t.Fatalf("解析 %q 失败: %v", expr, err)
		}
		next, ok := schedule.Next(time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)) // 周二
		if !ok || next.Weekday() != time.Sunday {
			t.Fatalf("%q 应落在周日，实际 %v", expr, next)
		}
	}
}
