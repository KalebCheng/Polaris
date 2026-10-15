import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import AsyncdPage from './asyncd-page'
import {
  asyncdApi,
  type AsyncdJob,
  type AsyncdJobRun,
  type AsyncdJobRunAttempt,
  type AsyncdWorker,
} from '../api'

/** 调度规则夹具（默认：每小时一次、已启用） */
function scheduleOf(over: Partial<NonNullable<AsyncdJob['schedule']>> = {}): NonNullable<AsyncdJob['schedule']> {
  return {
    kind: 'interval',
    interval_seconds: 3600,
    daily_times: [],
    window_minutes: 30,
    enabled: true,
    next_run_at: '2026-10-15T04:00:00Z',
    last_run_at: '2026-10-15T03:00:00Z',
    last_status: 'queued',
    ...over,
  }
}

/** 任务目录：2 个异步任务（其中 1 个从未执行过）+ 2 个定时任务 */
const jobs: AsyncdJob[] = [
  {
    id: 1,
    name: 'syncer.run',
    task_type: 'syncer.run',
    kind: 'async',
    stats: { total: 4, by_status: { succeeded: 3, dead: 1 }, last_queued_at: '2026-10-15T05:00:00Z' },
  },
  {
    id: 2,
    name: 'provision.account',
    task_type: 'provision.account',
    kind: 'async',
    stats: { total: 0, by_status: {}, last_queued_at: null },
  },
  {
    id: 3,
    name: 'maintenance.token_cleanup',
    task_type: 'maintenance.token_cleanup',
    kind: 'scheduled',
    schedule: scheduleOf(),
    stats: { total: 12, by_status: { succeeded: 12 }, last_queued_at: '2026-10-15T03:00:00Z' },
  },
  {
    id: 4,
    name: 'syncer.schedule',
    task_type: 'syncer.schedule',
    kind: 'scheduled',
    schedule: scheduleOf({
      kind: 'daily',
      daily_times: ['03:00'],
      enabled: false,
      last_status: 'skipped',
    }),
    stats: { total: 0, by_status: {}, last_queued_at: null },
  },
]

const workers: AsyncdWorker[] = [
  {
    id: 'host-1#0',
    instance_id: 'host-1',
    host: 'host-1',
    pid: 4242,
    slot_index: 0,
    state: 'online',
    started_at: '2026-10-15T02:00:00Z',
    last_heartbeat_at: '2026-10-15T05:00:00Z',
    running: 1,
  },
  {
    id: 'host-2#0',
    instance_id: 'host-2',
    host: 'host-2',
    pid: 99,
    slot_index: 0,
    state: 'stale',
    started_at: '2026-10-15T01:00:00Z',
    last_heartbeat_at: '2026-10-15T03:00:00Z',
    running: 0,
  },
]

/** syncer.run 的执行记录：1 次成功 + 1 条死信 */
const runs: AsyncdJobRun[] = [
  {
    id: 11,
    job_id: 1,
    job_name: 'syncer.run',
    task_type: 'syncer.run',
    status: 'succeeded',
    priority: 0,
    attempts: 1,
    max_attempts: 8,
    next_attempt_at: '2026-10-15T05:00:00Z',
    worker_id: 'host-1#0',
    created_at: '2026-10-15T05:00:00Z',
    updated_at: '2026-10-15T05:00:01Z',
    finished_at: '2026-10-15T05:00:01Z',
  },
  {
    id: 99,
    job_id: 1,
    job_name: 'syncer.run',
    task_type: 'syncer.run',
    status: 'dead',
    priority: 0,
    attempts: 8,
    max_attempts: 8,
    next_attempt_at: '2026-10-15T05:00:00Z',
    last_error: 'permission denied',
    created_at: '2026-10-15T04:00:00Z',
    updated_at: '2026-10-15T04:30:00Z',
    finished_at: '2026-10-15T04:30:00Z',
  },
]

/** 死信那次执行的两条尝试：一次普通失败 + 一次租约丢失 */
const attempts: AsyncdJobRunAttempt[] = [
  {
    id: 1,
    attempt_no: 1,
    worker_id: 'host-1#0',
    status: 'failed',
    started_at: '2026-10-15T04:00:00Z',
    finished_at: '2026-10-15T04:00:01Z',
    duration_ms: 1200,
    error: 'dial tcp: connection refused',
  },
  {
    id: 2,
    attempt_no: 2,
    worker_id: 'host-2#0',
    status: 'lease_lost',
    started_at: '2026-10-15T04:10:00Z',
    finished_at: '2026-10-15T04:11:00Z',
    duration_ms: 60000,
    error: 'worker lease expired',
  },
]

const runsMock = vi.fn(
  async (jobId: number, params?: { status?: string; page?: number; size?: number }) => {
    const { status, page, size } = params ?? {}
    const scoped = runs.filter((item) => item.job_id === jobId)
    const matched = status ? scoped.filter((item) => item.status === status) : scoped
    const pageSize = size ?? 10
    const offset = ((page ?? 1) - 1) * pageSize
    const counts: Record<string, number> = {}
    for (const item of scoped) counts[item.status] = (counts[item.status] ?? 0) + 1
    return { items: matched.slice(offset, offset + pageSize), counts, page: page ?? 1, size: pageSize }
  },
)

vi.mock('../api', () => ({
  asyncdApi: {
    jobs: async () => ({ items: jobs }),
    workers: async () => ({ items: workers }),
    jobRuns: (jobId: number, params?: { status?: string; page?: number; size?: number }) =>
      runsMock(jobId, params),
    jobRunAttempts: vi.fn(async () => ({ items: attempts })),
    retryJobRun: vi.fn(async () => undefined),
    setJobEnabled: vi.fn(async () => undefined),
  },
}))

/** 取任务卡（卡片都是 article，用卡内的任务名定位） */
function jobCard(name: string): HTMLElement {
  const card = screen.getAllByRole('article').find((element) => element.textContent?.includes(name) ?? false)
  if (!card) throw new Error(`找不到任务卡片: ${name}`)
  return card
}

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <AsyncdPage />
    </I18nextProvider>,
  )
}

/**
 * 任务中心：四个维度各有单独的呈现方式。
 * 这些用例锁定"两个任务维度分开、从未执行过的任务也在、执行记录按任务过滤、
 * 尝试明细可展开、执行器在线状态可读"这几条核心口径。
 */
describe('任务中心（任务维度 + 执行记录 + 执行器）', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('默认展示异步任务维度：任务类型、事件驱动徽标与执行统计都在卡片上', async () => {
    renderPage()
    await screen.findAllByText('syncer.run')

    const card = jobCard('syncer.run')
    expect(within(card).getByText('事件驱动')).toBeInTheDocument()
    expect(within(card).getByText('累计 4 次')).toBeInTheDocument()
    expect(within(card).getByText('成功').nextSibling?.textContent).toBe('3')
    expect(within(card).getByText('死信').nextSibling?.textContent).toBe('1')
    // 从未执行过的任务同样出现（否则"没跑过"与"任务不存在"无法区分）
    expect(within(jobCard('provision.account')).getByText('尚未执行')).toBeInTheDocument()
    // 异步维度里不应出现定时任务
    expect(screen.queryByText('maintenance.token_cleanup')).toBeNull()
  })

  it('切到定时任务维度：频率、停用状态与启停开关', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findAllByText('syncer.run')

    await user.click(screen.getByRole('tab', { name: '定时任务' }))

    const cleanup = jobCard('maintenance.token_cleanup')
    expect(within(cleanup).getByText('周期调度')).toBeInTheDocument()
    expect(within(cleanup).getByText('每 3600 秒')).toBeInTheDocument()
    // 停用的调度项：不再显示"下次执行"时间，而是如实说"已停用"
    const scan = jobCard('syncer.schedule')
    expect(within(scan).getByText('每天 03:00 · 窗口 30 分钟')).toBeInTheDocument()
    expect(within(scan).getByText('已停用')).toBeInTheDocument()

    await user.click(within(cleanup).getByRole('switch'))
    await waitFor(() => expect(asyncdApi.setJobEnabled).toHaveBeenCalledWith(3, false))
  })

  it('执行记录：按任务过滤、死信可重试、可展开尝试明细', async () => {
    const user = userEvent.setup()
    runsMock.mockClear()
    renderPage()
    await screen.findAllByText('syncer.run')

    await user.click(within(jobCard('syncer.run')).getByRole('button', { name: '执行记录' }))
    // 归属由路径表达：执行记录挂在任务定义下（/asyncd/jobs/:id/runs），不会把所有任务的执行混在一起
    await waitFor(() => expect(runsMock).toHaveBeenCalledWith(1, expect.anything()))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('permission denied')).toBeInTheDocument()

    // 死信行内可重试
    await user.click(within(dialog).getByRole('button', { name: '重试' }))
    await waitFor(() => expect(asyncdApi.retryJobRun).toHaveBeenCalledWith(1, 99))

    // 尝试明细：第 1 次失败（1.2s）+ 第 2 次租约丢失（1m0s）
    await user.click(within(dialog).getAllByRole('button', { name: '尝试明细' })[1])
    await waitFor(() => expect(asyncdApi.jobRunAttempts).toHaveBeenCalledWith(1, 99))
    await waitFor(() => expect(within(dialog).getByText('执行 #99 的尝试明细')).toBeInTheDocument())
    expect(within(dialog).getByText('第 1 次')).toBeInTheDocument()
    expect(within(dialog).getByText('租约丢失')).toBeInTheDocument()
    expect(within(dialog).getByText('dial tcp: connection refused')).toBeInTheDocument()
    expect(within(dialog).getByText('1.2s')).toBeInTheDocument()
  })

  it('执行器维度：实例、槽位、在线状态与在跑任务数', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findAllByText('syncer.run')

    await user.click(screen.getByRole('tab', { name: '执行器' }))

    expect(await screen.findByText('host-1')).toBeInTheDocument()
    expect(screen.getByText('host-2')).toBeInTheDocument()
    // 心跳新鲜 = 在线；心跳过期 = 已失联（由后端按心跳算出）
    expect(screen.getByText('在线')).toBeInTheDocument()
    expect(screen.getByText('已失联')).toBeInTheDocument()
    expect(screen.getByText(/个执行器/)).toBeInTheDocument()
  })
})
