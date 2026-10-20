import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { NavPills } from './nav-pills'

const items = [
  { to: '/', label: '总览', active: false },
  { to: '/roles', label: '角色', active: true },
  { to: '/audit', label: '审计', active: false },
]

function renderPills() {
  return render(
    <MemoryRouter>
      <NavPills items={items} ariaLabel="平台" />
    </MemoryRouter>,
  )
}

/**
 * 顶栏导航滑块：选中态是**一块滑动的墨色胶囊**，不是每个按钮自己点亮。
 *
 * 这条契约值得锁：如果谁把 `bg-ink` 加回具体条目，画面上就会出现"两层胶囊"——
 * 一块动的、一块不动的，切换时看着像重影（该滑块与 `.sliding-pill-indicator` 是一套）。
 */
describe('NavPills 导航滑块', () => {
  it('渲染唯一一块滑块，且对读屏隐藏', () => {
    const { container } = renderPills()

    const indicators = container.querySelectorAll('.sliding-pill-indicator')
    expect(indicators).toHaveLength(1)
    expect(indicators[0]).toHaveAttribute('aria-hidden', 'true')
  })

  it('选中项只改文字色，不自己画墨色底（底色由滑块负责）', () => {
    renderPills()

    const active = screen.getByRole('link', { name: '角色' })
    expect(active.className).toContain('text-ink-foreground')
    expect(active.className).not.toContain('bg-ink')
    expect(active).toHaveAttribute('data-active', 'true')

    const inactive = screen.getByRole('link', { name: '总览' })
    expect(inactive.className).not.toContain('text-ink-foreground')
    expect(inactive).toHaveAttribute('data-active', 'false')
  })

  it('无选中项时不渲染滑块（例如当前路径不属于该导航）', () => {
    const { container } = render(
      <MemoryRouter>
        <NavPills items={items.map((item) => ({ ...item, active: false }))} ariaLabel="平台" />
      </MemoryRouter>,
    )

    expect(container.querySelectorAll('.sliding-pill-indicator')).toHaveLength(0)
  })

  it('横向滚动时滚动容器与动画裁剪层分离，避免滑块过冲撑出滚动条', () => {
    render(
      <MemoryRouter>
        <NavPills items={items} ariaLabel="平台" scrollable />
      </MemoryRouter>,
    )

    const nav = screen.getByRole('navigation', { name: '平台' })
    const scroller = nav.parentElement
    expect(scroller).not.toBeNull()
    expect(scroller).toHaveClass('overflow-x-auto')
    expect(nav).toHaveClass('overflow-x-clip')
    expect(nav).not.toHaveClass('overflow-x-auto')
  })
})
