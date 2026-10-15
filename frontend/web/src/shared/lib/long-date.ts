/**
 * 长日期（含星期）格式化：控制台首页页头的「星期一，2025年9月8日」。
 *
 * 走 `Intl.DateTimeFormat` 而不是手写模板——中文的"年/月/日 + 星期"与英文的
 * "Thursday, October 15" 语序不同，手写必然在一门语言里别扭。
 */
export function formatLongDate(value: Date | string | number = new Date(), locale = 'zh-CN'): string {
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat(locale, {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  }).format(date)
}

/** 按时段给出问候语 key（早晨 / 下午 / 晚间三段，避免出现"凌晨好"这类别扭文案） */
export function greetingPeriodKey(hour: number): 'dashboard.morning' | 'dashboard.afternoon' | 'dashboard.evening' {
  if (hour < 12) return 'dashboard.morning'
  if (hour < 18) return 'dashboard.afternoon'
  return 'dashboard.evening'
}
