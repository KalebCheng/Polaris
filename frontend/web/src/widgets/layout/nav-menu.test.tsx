import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import zh from '@/shared/i18n/locales/zh.json'
import { WorkspaceNav } from './nav-list'

/** 只关心"菜单怎么画"，权限与组织上下文给足即可 */
const state = vi.hoisted(() => ({ institutionId: 3 }))

vi.mock('@/features/auth/auth-context', () => ({ useAuth: () => ({ can: () => true }) }))
vi.mock('@/features/iam/institution-context', () => ({
  useCurrentInstitution: () => ({ institutionId: state.institutionId }),
}))

function renderNav(path = '/institutions/3/users') {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[path]}>
        <WorkspaceNav />
      </MemoryRouter>
    </I18nextProvider>,
  )
}

/**
 * 组织管理的资源侧栏与平台二级菜单是**同一份实现**（`NavMenu` → `NavPills orientation="vertical"`）。
 *
 * 这里锁两件事：
 *  1) 侧栏的选中态也变成了那块**会滑的墨色胶囊**（不再是"每个条目自己染主色底"的第三套写法）；
 *  2) 条目仍然指向工作区地址、总览用精确匹配（否则 `/institutions/3/users` 会把总览也点亮）。
 */
describe('工作区侧栏（共享纵向菜单）', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('选中项是唯一一块滑动墨色胶囊，不再自带主色底', () => {
    const { container } = renderNav()

    expect(container.querySelectorAll('.sliding-pill-indicator')).toHaveLength(1)

    const active = screen.getByRole('link', { name: zh.nav.users })
    expect(active).toHaveAttribute('data-active', 'true')
    expect(active).toHaveAttribute('href', '/institutions/3/users')
    expect(active.className).toContain('text-ink-foreground')
    // 底色由滑块负责：自己再画一层会与滑块形成重影（顶栏同口径）
    expect(active.className).not.toContain('bg-ink')
    expect(active.className).not.toContain('bg-primary/10')

    expect(screen.getByRole('link', { name: zh.nav.groups })).toHaveAttribute('data-active', 'false')
  })

  it('总览用精确匹配：子页路径下不点亮总览，回到组织首页才点亮', () => {
    renderNav('/institutions/3')
    expect(screen.getByRole('link', { name: zh.nav.overview })).toHaveAttribute('data-active', 'true')
    // 工作区里的"应用"与总览是两个条目，互不串台
    expect(screen.getByRole('link', { name: zh.nav.applications })).toHaveAttribute('data-active', 'false')
  })

  it('还没选组织时整列禁用，且不点亮任何条目（滑块不渲染）', () => {
    state.institutionId = 0
    try {
      const { container } = renderNav('/institutions')
      expect(container.querySelectorAll('.sliding-pill-indicator')).toHaveLength(0)
      expect(screen.queryByRole('link', { name: zh.nav.users })).toBeNull()
      // 禁用态下条目仍在（灰显），但不是可跳转链接
      expect(screen.getByText(zh.institutions.selectHint)).toBeInTheDocument()
    } finally {
      state.institutionId = 3
    }
  })

  it('分组抬头仍在（菜单列表只替换条目区，不吞掉分组结构）', () => {
    renderNav()
    // 分组抬头是可折叠按钮，文案取自 nav-config 的 nav.groupOverview / nav.directory / nav.apps / nav.lifecycle
    for (const label of [zh.nav.groupOverview, zh.nav.directory, zh.nav.apps, zh.nav.lifecycle]) {
      expect(screen.getByRole('button', { name: new RegExp(label) })).toBeInTheDocument()
    }
  })
})

/**
 * 跨分组滑动：整块菜单只有**一个容器**，分组抬头只是"不可选中的条目"。
 *
 * 关键回归：早先每个分组各渲染一份列表 → 跨分组时旧滑块卸载、新滑块就位，看着像没有动效。
 * 这里锁"切换后仍是同一个 DOM 节点，且位移真的变了"。
 */
describe('工作区侧栏：跨分组切换时滑块跟着移动', () => {
  /** 给 jsdom 装上"有尺寸"的矩形：容器与条目各给一套纵向坐标 */
  function stubLayout() {
    const original = Element.prototype.getBoundingClientRect
    const box = (left: number, top: number, width: number, height: number) =>
      ({ left, top, width, height, right: left + width, bottom: top + height, x: left, y: top }) as DOMRect
    Element.prototype.getBoundingClientRect = function (this: HTMLElement) {
      if (this.tagName === 'NAV') return box(20, 60, 240, 600)
      const label = this.textContent ?? ''
      // 「概览」在第一个分组（y=64），「用户」在第二个分组（y=240，跨了两个抬头）
      if (label === zh.nav.overview) return box(24, 64, 232, 36)
      if (label === zh.nav.users) return box(24, 240, 232, 36)
      return box(0, 0, 0, 0)
    }
    return () => {
      Element.prototype.getBoundingClientRect = original
    }
  }

  it('滑块是同一个 DOM 节点，位移从第一组移到第二组', async () => {
    const restore = stubLayout()
    try {
      const user = userEvent.setup()
      const { container } = renderNav('/institutions/3')

      const before = container.querySelector('.sliding-pill-indicator') as HTMLElement
      expect(before.style.transform).toBe('translate(4px, 4px)')

      await user.click(screen.getByRole('link', { name: zh.nav.users }))

      const after = container.querySelector('.sliding-pill-indicator') as HTMLElement
      expect(after).toBe(before) // 同一个节点 → CSS 过渡才有起点
      expect(after.style.transform).toBe('translate(4px, 180px)')
    } finally {
      restore()
    }
  })
})
