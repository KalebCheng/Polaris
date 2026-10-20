import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { Combobox, MultiSelect } from './multi-select'

const options = [
  { value: 'AtLeast6', label: '至少 6 位' },
  { value: 'AtLeast8', label: '至少 8 位' },
]

/**
 * 回归（真实缺陷）：下拉内容必须 Portal 到 body。
 *
 * 历史问题：`PopoverContent` 没走 `Popover.Portal`，浮层被渲染在表单卡片内部，
 * 而卡片是 `surface overflow-hidden`（`backdrop-filter` 还会新建层叠上下文），
 * 于是密码强度策略的下拉被卡片裁掉、被下一张卡片盖住，只露出一截搜索框。
 */
describe('下拉浮层的挂载点', () => {
  it('MultiSelect 的下拉渲染到 body，不被 overflow-hidden 容器裁切', async () => {
    render(
      <div data-testid="clipper" className="overflow-hidden">
        <MultiSelect options={options} value={[]} onChange={vi.fn()} />
      </div>,
    )
    const clipper = screen.getByTestId('clipper')

    await userEvent.click(screen.getByRole('combobox'))
    const search = await screen.findByPlaceholderText('搜索…')

    expect(clipper.contains(search)).toBe(false)
    expect(document.body.contains(search)).toBe(true)
  })

  it('Combobox 的下拉同样渲染到 body', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <div data-testid="clipper" className="overflow-hidden">
          <Combobox options={options} value="" onChange={vi.fn()} />
        </div>
      </I18nextProvider>,
    )
    const clipper = screen.getByTestId('clipper')

    await userEvent.click(screen.getByRole('combobox'))
    const search = await screen.findByPlaceholderText('搜索…')

    expect(clipper.contains(search)).toBe(false)
    expect(document.body.contains(search)).toBe(true)
  })
})
