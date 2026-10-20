import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { ListPagination } from './list-pagination'

function renderPagination(page = 3) {
  return render(
    <I18nextProvider i18n={i18n}>
      <ListPagination page={page} size={15} total={200} onPageChange={vi.fn()} />
    </I18nextProvider>,
  )
}

describe('ListPagination', () => {
  // 回归：页码切换的选中态是"一块滑动的墨色胶囊"（与顶栏/分段控件共用 SlidingPill），
  // 选中项自己**不再**加 bg-ink——否则会出现"一块动的 + 一块不动的"重影。
  it('当前页由滑动胶囊表达，而不是自己染底色', async () => {
    renderPagination(3)
    const current = await screen.findByRole('button', { name: '3' })
    expect(current).toHaveAttribute('data-active', 'true')
    expect(current.className).not.toContain('bg-ink')

    const pill = document.querySelector('.sliding-pill-indicator')
    expect(pill).not.toBeNull()
  })

  it('总数与范围文案按每页 15 条计算', async () => {
    renderPagination(2)
    // 第 2 页、每页 15 条 → 16–30
    expect(await screen.findByText(/16/)).toBeInTheDocument()
  })
})
