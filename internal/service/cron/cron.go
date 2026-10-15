// Package cron 实现标准 5 段 Cron 表达式的解析与"下一次触发时间"计算。
//
// 支持：`*`、具体数值、列表（1,2,3）、区间（1-5）、步长（*/5、1-30/2）；
// 段顺序为「分 时 日 月 周」。刻意不引入第三方依赖：表达式只在"定时校验"这一处使用，
// 自带实现更小、更可控（也便于把错误信息做成可读的中文提示）。
//
// 计算方式：从参考时间起逐分钟向后扫描（上限 366 天），命中即返回。
// 对"分钟级"调度足够，且不会出现花哨实现里的边界错误。
package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// maxScanMinutes 向后扫描上限（366 天）：超时视为该表达式在一年内不会触发
const maxScanMinutes = 366 * 24 * 60

// Schedule 解析后的 5 段表达式
type Schedule struct {
	minute, hour, dayOfMonth, month, dayOfWeek fieldSet
}

// fieldSet 某一段允许的取值集合
type fieldSet struct {
	allowed    [64]bool // 值域最大 0-59
	max        int
	restricted bool // 是否不是 `*`（用于"日/周同时限制时取或"的语义）
}

// Parse 解析表达式；不合法时返回可读错误
func Parse(expr string) (*Schedule, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron 表达式应为 5 段（分 时 日 月 周），实际 %d 段", len(parts))
	}
	limits := [5]int{59, 23, 31, 12, 7} // 周：0-7（0 与 7 都表示周日）
	sets := make([]fieldSet, 5)
	for i, part := range parts {
		set, err := parseField(part, limits[i])
		if err != nil {
			return nil, fmt.Errorf("cron 第 %d 段 %q 非法: %w", i+1, part, err)
		}
		sets[i] = set
	}
	// 周日的两种写法归一：7 → 0
	if sets[4].allowed[7] {
		sets[4].allowed[0] = true
	}
	return &Schedule{
		minute: sets[0], hour: sets[1], dayOfMonth: sets[2], month: sets[3], dayOfWeek: sets[4],
	}, nil
}

// parseField 解析单段
func parseField(part string, max int) (fieldSet, error) {
	set := fieldSet{max: max}
	if part == "" {
		return set, errors.New("空段")
	}
	if part != "*" {
		set.restricted = true
	}
	for _, item := range strings.Split(part, ",") {
		if err := parseItem(&set, strings.TrimSpace(item), max); err != nil {
			return set, err
		}
	}
	if !set.any() {
		return set, errors.New("未产生任何取值")
	}
	return set, nil
}

func parseItem(set *fieldSet, item string, max int) error {
	step := 1
	if slash := strings.Index(item, "/"); slash >= 0 {
		parsed, err := strconv.Atoi(item[slash+1:])
		if err != nil || parsed <= 0 {
			return fmt.Errorf("步长 %q 非法", item[slash+1:])
		}
		step = parsed
		item = item[:slash]
	}
	low, high := 0, max
	if item != "*" {
		if dash := strings.Index(item, "-"); dash >= 0 {
			parsedLow, err := strconv.Atoi(item[:dash])
			if err != nil {
				return fmt.Errorf("区间起点 %q 非法", item[:dash])
			}
			parsedHigh, err := strconv.Atoi(item[dash+1:])
			if err != nil {
				return fmt.Errorf("区间终点 %q 非法", item[dash+1:])
			}
			low, high = parsedLow, parsedHigh
		} else {
			value, err := strconv.Atoi(item)
			if err != nil {
				return fmt.Errorf("取值 %q 非法", item)
			}
			// 单值 + 步长（如 "5/10"）按标准解释为"从 5 开始到上界"
			low, high = value, max
			if step == 1 {
				high = value
			}
		}
	}
	if low < 0 || high > max || low > high {
		return fmt.Errorf("取值超出范围 0-%d", max)
	}
	for value := low; value <= high; value += step {
		set.allowed[value] = true
	}
	return nil
}

func (s fieldSet) any() bool {
	for _, ok := range s.allowed {
		if ok {
			return true
		}
	}
	return false
}

func (s fieldSet) has(value int) bool {
	if value < 0 || value >= len(s.allowed) {
		return false
	}
	return s.allowed[value]
}

// Next 返回严格晚于 after 的下一次触发时间；一年内不会触发时返回零值与 false
func (s *Schedule) Next(after time.Time) (time.Time, bool) {
	candidate := after.Truncate(time.Minute).Add(time.Minute)
	loc := after.Location()
	for i := 0; i < maxScanMinutes; i++ {
		current := candidate.Add(time.Duration(i) * time.Minute)
		if !s.month.has(int(current.Month())) {
			continue
		}
		// 标准 cron 语义：日与周都受限时取"或"，否则取"与"
		dayMatch := s.dayOfMonth.has(current.Day())
		weekMatch := s.dayOfWeek.has(int(current.Weekday()))
		if s.dayOfMonth.restricted && s.dayOfWeek.restricted {
			if !dayMatch && !weekMatch {
				continue
			}
		} else if !dayMatch || !weekMatch {
			continue
		}
		if !s.hour.has(current.Hour()) || !s.minute.has(current.Minute()) {
			continue
		}
		return time.Date(current.Year(), current.Month(), current.Day(),
			current.Hour(), current.Minute(), 0, 0, loc), true
	}
	return time.Time{}, false
}
