import { afterEach, describe, expect, it, vi } from 'vitest'
import { formatRelative, formatRelativeFuture } from './format'

describe('formatRelativeFuture', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('未来时间按方向给出"X 后"，不再一律显示"刚刚"', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-22T12:00:00Z'))
    expect(formatRelativeFuture('2026-10-22T12:00:30Z')).toBe('即将执行')
    expect(formatRelativeFuture('2026-10-22T12:59:00Z')).toBe('59 分钟后')
    expect(formatRelativeFuture('2026-10-22T13:00:00Z')).toBe('1 小时后')
    expect(formatRelativeFuture('2026-10-23T12:00:00Z')).toBe('1 天后')
  })

  it('已过期时暴露真实逾期时长，而不是被"刚刚"掩盖', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-22T12:00:00Z'))
    expect(formatRelativeFuture('2026-10-22T11:59:30Z')).toBe('待执行')
    expect(formatRelativeFuture('2026-10-22T11:30:00Z')).toBe('已逾期 30 分钟')
    expect(formatRelativeFuture('2026-10-22T09:00:00Z')).toBe('已逾期 3 小时')
  })

  it('回归：过去时间不能用它（方向相反）——这是两者必须分开的原因', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-22T12:00:00Z'))
    // formatRelative 用于过去是对的
    expect(formatRelative('2026-10-22T11:30:00Z')).toBe('30 分钟前')
    // 而把未来时间交给它，会落进 diff < minute 分支、永远"刚刚"（本次修的就是这个误用）
    expect(formatRelative('2026-10-22T13:00:00Z')).toBe('刚刚')
  })
})
