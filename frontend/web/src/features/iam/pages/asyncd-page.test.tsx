import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import AsyncdPage from './asyncd-page'
import { asyncdApi, type AsyncdTask } from '../api'

/** 任务夹具（只填与用例相关的字段） */
function task(over: Partial<AsyncdTask>): AsyncdTask {
  return {
    id: 1,
    type: 'provision.account',
    status: 'pending',
    priority: 0,
    attempts: 1,
    max_attempts: 5,
    next_attempt_at: '2026-10-15T02:00:00Z',
    created_at: '2026-10-15T01:00:00Z',
    updated_at: '2026-10-15T01:00:00Z',
    ...over,
  }
}

/** 20 条 pending + 1 条死信：用于验证分页与"换筛选回第 1 页" */
const allTasks: AsyncdTask[] = [
  ...Array.from({ length: 20 }, (_, index) => task({ id: index + 1, unique_key: `key-${index + 1}` })),
  task({ id: 99, status: 'archived', unique_key: 'dead-1', last_error: 'permission denied' }),
]

const tasksMock = vi.fn(async ({ status, page, size }: { status?: string; page?: number; size?: number }) => {
  const matched = status ? allTasks.filter((item) => item.status === status) : allTasks
  const offset = ((page ?? 1) - 1) * (size ?? 12)
  return {
    items: matched.slice(offset, offset + (size ?? 12)),
    counts: { pending: 20, archived: 1 },
    page: page ?? 1,
    size: size ?? 12,
  }
})

vi.mock('../api', () => ({
  asyncdApi: {
    tasks: (params: { status?: string; page?: number; size?: number }) => tasksMock(params),
    schedules: async () => ({
      items: [
        {
          id: 1,
          job_name: 'asyncd:cleanup',
          task_type: 'asyncd.cleanup',
          kind: 'interval',
          interval_seconds: 3600,
          daily_times: null,
          window_minutes: 0,
          enabled: true,
          next_run_at: '2026-10-15T03:00:00Z',
          last_run_at: '2026-10-15T02:00:00Z',
          last_status: 'enqueued',
        },
      ],
    }),
    retry: vi.fn(async () => undefined),
    setScheduleEnabled: vi.fn(async () => undefined),
  },
}))

/** 任务卡（调度卡也是 article，用"卡内含任务类型"把两者分开） */
function taskCards(): HTMLElement[] {
  return screen
    .getAllByRole('article')
    .filter((card) => card.textContent?.includes('provision.account') ?? false)
}

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <AsyncdPage />
    </I18nextProvider>,
  )
}

/**
 * 任务中心与组织管理下的资源页同构：筛选行 + 卡片网格 + 分页。
 * 这些用例锁住"卡片化之后仍然能一眼看清任务、仍然能翻页与人工干预"。
 */
describe('任务中心（卡片网格）', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('任务是卡片：类型 / 状态 / 尝试次数 / 唯一键都在卡片里，只有死信才有「重试」', async () => {
    renderPage()
    await screen.findAllByText('provision.account')
    const cards = taskCards()
    // 第 1 页 12 张（总 21 条，counts 求和派生出分页总数）
    expect(cards).toHaveLength(12)
    const first = cards[0]
    expect(within(first).getByText('provision.account')).toBeInTheDocument()
    expect(within(first).getByText('等待中')).toBeInTheDocument()
    expect(within(first).getByText('第 1/5 次')).toBeInTheDocument()
    expect(within(first).getByText('key-1')).toBeInTheDocument()
    // 第 1 页里没有死信 → 没有重试按钮
    expect(screen.queryByRole('button', { name: '重试' })).toBeNull()
    // 分页总数来自后端状态直方图（20 + 1）
    expect(screen.getByText(/共 21 条/)).toBeInTheDocument()
  })

  it('翻页与切换状态筛选：筛选变化必须回到第 1 页', async () => {
    const user = userEvent.setup()
    tasksMock.mockClear()
    renderPage()
    await screen.findAllByText('provision.account')

    await user.click(screen.getByRole('button', { name: '下一页' }))
    await waitFor(() => expect(tasksMock).toHaveBeenCalledWith(expect.objectContaining({ page: 2 })))

    await user.click(screen.getByRole('tab', { name: '死信' }))
    await waitFor(() =>
      expect(tasksMock).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'archived', page: 1 })),
    )
    // 死信卡：显示错误原因 + 「重试」
    await waitFor(() => expect(taskCards()).toHaveLength(1))
    const cards = taskCards()
    expect(within(cards[0]).getByText('permission denied')).toBeInTheDocument()
    expect(within(cards[0]).getByRole('button', { name: '重试' })).toBeInTheDocument()

    await user.click(within(cards[0]).getByRole('button', { name: '重试' }))
    await waitFor(() => expect(asyncdApi.retry).toHaveBeenCalledWith(99))
  })

  it('定时调度也是卡片：显示频率与下次执行，开关可停用', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('asyncd:cleanup')

    expect(screen.getByText('asyncd:cleanup')).toBeInTheDocument()
    expect(screen.getByText('每 3600 秒')).toBeInTheDocument()

    await user.click(screen.getByRole('switch'))
    await waitFor(() => expect(asyncdApi.setScheduleEnabled).toHaveBeenCalledWith('asyncd:cleanup', false))
  })
})
