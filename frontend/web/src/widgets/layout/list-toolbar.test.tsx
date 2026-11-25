import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { ListToolbar } from './list-toolbar'

describe('ListToolbar', () => {
  // 回归：部门候选很长时，下拉必须限制在视口可用高度内并允许滚动，
  // 否则超出页面底部的选项既看不见也点不到。
  it('长筛选项下拉限制高度并滚动', async () => {
    const user = userEvent.setup()
    render(
      <I18nextProvider i18n={i18n}>
        <ListToolbar
          keyword=""
          onKeywordChange={vi.fn()}
          filters={[
            {
              label: 'Department',
              value: '',
              options: Array.from({ length: 40 }, (_, index) => ({
                value: String(index + 1),
                label: `Department ${index + 1}`,
              })),
              onChange: vi.fn(),
            },
          ]}
        />
      </I18nextProvider>,
    )

    await user.click(screen.getByRole('button', { name: /Department/ }))
    const menu = await screen.findByRole('menu')
    expect(menu.className).toContain('overflow-y-auto')
    expect(menu.className).toContain('--radix-dropdown-menu-content-available-height')
  })
})
