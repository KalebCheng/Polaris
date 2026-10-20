import { describe, expect, it } from 'vitest'
import { formatLongDate, greetingPeriodKey } from './long-date'

/** 控制台首页页头：问候语时段 + 长日期 */
describe('formatLongDate', () => {
  it('按语言给出含星期的长日期', () => {
    const date = new Date(2025, 8, 8, 9, 0, 0) // 2025-09-08 星期一
    expect(formatLongDate(date, 'zh-CN')).toContain('星期一')
    expect(formatLongDate(date, 'zh-CN')).toContain('2025年9月8日')
    expect(formatLongDate(date, 'en-US')).toContain('Monday')
  })

  it('无法解析的入参回退为占位符，不抛异常', () => {
    expect(formatLongDate('not-a-date', 'zh-CN')).toBe('-')
  })
})

describe('greetingPeriodKey', () => {
  it('按时段给出问候语词条', () => {
    expect(greetingPeriodKey(8)).toBe('dashboard.morning')
    expect(greetingPeriodKey(15)).toBe('dashboard.afternoon')
    expect(greetingPeriodKey(21)).toBe('dashboard.evening')
    // 凌晨也走"早上好"，不出现"凌晨好"这类别扭文案
    expect(greetingPeriodKey(3)).toBe('dashboard.morning')
  })
})
