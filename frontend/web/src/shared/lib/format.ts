const pad = (n: number) => String(n).padStart(2, '0')

/** Format a timestamp in local time. */
export function formatDateTime(value?: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '-'
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** 相对时间：用于"最近登录""最后同步"等场景 */
export function formatRelative(value?: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '-'
  const diff = Date.now() - d.getTime()
  const minute = 60_000
  const hour = 60 * minute
  const day = 24 * hour
  if (diff < minute) return '刚刚'
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`
  if (diff < 30 * day) return `${Math.floor(diff / day)} 天前`
  return formatDateTime(value)
}

/**
 * 未来时间的相对描述：用于"下次执行"这类**指向未来**的时间点。
 *
 * 与 formatRelative 的分工必须分清：那个是 `now - value`，只适用于过去的时间点。
 * 未来时间若误用它，diff 为负 → 落进 `diff < minute` → **永远显示"刚刚"**
 * （2026-10-22 实测：每小时的任务也显示"下次执行 刚刚"，与"最近入队 26 分钟前"自相矛盾）。
 *
 * 已过期时返回"已逾期 X"：让"到点却没被领取"暴露出来，而不是被"刚刚"掩盖。
 */
export function formatRelativeFuture(value?: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '-'
  const diff = d.getTime() - Date.now()
  const minute = 60_000
  const hour = 60 * minute
  const day = 24 * hour
  if (diff <= 0) {
    const overdue = -diff
    if (overdue < minute) return '待执行'
    if (overdue < hour) return `已逾期 ${Math.floor(overdue / minute)} 分钟`
    if (overdue < day) return `已逾期 ${Math.floor(overdue / hour)} 小时`
    return `已逾期 ${Math.floor(overdue / day)} 天`
  }
  if (diff < minute) return '即将执行'
  if (diff < hour) return `${Math.ceil(diff / minute)} 分钟后`
  if (diff < day) return `${Math.ceil(diff / hour)} 小时后`
  if (diff < 30 * day) return `${Math.ceil(diff / day)} 天后`
  return formatDateTime(value)
}

/** 布尔值转审阅友好的文案 */
export function yesNo(value: boolean | undefined, yes = '是', no = '否'): string {
  return value ? yes : no
}
