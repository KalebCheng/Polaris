import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeAll, describe, expect, it } from 'vitest'
import { Button } from '@/shared/ui'
import i18n from '@/shared/i18n'
import zh from '@/shared/i18n/locales/zh.json'
import { usePageHeaderActions } from '../components/page-header-actions'
import { SecurityLayout } from './security-layout'
import { SettingsLayout } from './settings-layout'

const box = (left: number, top: number, width: number, height: number) =>
  ({ left, top, width, height, right: left + width, bottom: top + height, x: left, y: top }) as DOMRect

/** 桩页面：内容一行 + 可选页头操作（真实页面里分别是列表、新建弹窗、刷新按钮） */
function stubPage(label: string, withActions = false) {
  return function StubPage() {
    const headerActions = usePageHeaderActions(
      withActions ? (
        <Button type="button">
          {zh.certs.title}
          {zh.common.create}
        </Button>
      ) : null,
    )
    return (
      <div>
        {headerActions}
        {label}
      </div>
    )
  }
}

const CertsStub = stubPage('证书内容', true)
const TokensStub = stubPage('令牌内容')
const SessionsStub = stubPage('会话内容')
const StorageStub = stubPage('存储内容')
const TasksStub = stubPage('任务内容')
const AuditStub = stubPage('审计内容')

/**
 * 给 jsdom 装上"有尺寸"的矩形（默认全 0 → 滑块位置恒为 0，锁不住"会不会跟着动"）。
 * 容器刻意放在 (40, 100)：滑块用**两者 rect 之差**，期望值因此是 4px 而非绝对坐标。
 */
function stubLayout() {
  const original = Element.prototype.getBoundingClientRect
  Element.prototype.getBoundingClientRect = function (this: HTMLElement) {
    if (this.tagName === 'NAV') return box(40, 100, 260, 36)
    const label = this.textContent ?? ''
    if (label === zh.nav.certs) return box(44, 104, 90, 32)
    if (label === zh.nav.tokens) return box(134, 104, 90, 32)
    if (label === zh.nav.sessions) return box(224, 104, 90, 32)
    // 系统设置组：纵向菜单（每行满宽，行高 36、间距 0），容器是 (40,100,240,120)
    if (label === zh.storage.title) return box(44, 104, 232, 36)
    if (label === zh.nav.asyncd) return box(44, 140, 232, 36)
    if (label === zh.nav.audit) return box(44, 176, 232, 36)
    return box(0, 0, 0, 0)
  }
  return () => {
    Element.prototype.getBoundingClientRect = original
  }
}

function renderSecurity(path = '/security/certs') {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/security" element={<SecurityLayout />}>
            <Route path="certs" element={<CertsStub />} />
            <Route path="tokens" element={<TokensStub />} />
            <Route path="sessions" element={<SessionsStub />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

function renderSettings(path = '/settings/storage') {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/settings" element={<SettingsLayout />}>
            <Route path="storage" element={<StorageStub />} />
            <Route path="tasks" element={<TasksStub />} />
            <Route path="audit" element={<AuditStub />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

/**
 * 二级页的组级布局：页头与标签常驻，页面只提供内容。
 *
 * 这里锁的是"**能滑**"的前提——标签必须跨路由切换保持**同一个 DOM 元素**：
 * CSS transition 只能过渡同一元素的属性，元素一旦重建（各页自己渲染标签时就是这样），
 * 滑块每次都是"新元素落在新位置"，只能瞬时出现。回归时这条会失败。
 */
describe('二级页组级布局（安全管理 / 系统设置）', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('页头与标签由布局渲染：标题、描述、三个标签、唯一滑块', () => {
    const { container } = renderSecurity()

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(zh.certs.title)
    expect(screen.getByText(zh.certs.platformSubtitle)).toBeInTheDocument()
    expect(screen.getAllByRole('link')).toHaveLength(3)
    expect(container.querySelectorAll('.sliding-pill-indicator')).toHaveLength(1)
    // 选中项只改文字色，底色一律交给滑块
    for (const link of screen.getAllByRole('link')) {
      expect(link.className).not.toContain('bg-ink')
    }
  })

  it('点另一个二级页：滑块是同一个 DOM 节点，位置随之重算（"能滑"的前提）', async () => {
    const restore = stubLayout()
    try {
      const user = userEvent.setup()
      const { container } = renderSecurity()
      const before = container.querySelector('.sliding-pill-indicator') as HTMLElement
      expect(before.style.transform).toBe('translate(4px, 4px)')

      await user.click(screen.getByRole('link', { name: zh.nav.tokens }))

      const after = container.querySelector('.sliding-pill-indicator') as HTMLElement
      // 关键：标签没被卸载重建，CSS 过渡才有起点
      expect(after).toBe(before)
      expect(after.style.transform).toBe('translate(94px, 4px)')
    } finally {
      restore()
    }
  })

  it('切换二级页时页头文案与内容跟着换', async () => {
    const user = userEvent.setup()
    renderSecurity()

    expect(screen.getByText('证书内容')).toBeInTheDocument()
    await user.click(screen.getByRole('link', { name: zh.nav.sessions }))

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(zh.sessions.title)
    expect(screen.getByText(zh.sessions.platformSubtitle)).toBeInTheDocument()
    expect(screen.queryByText('证书内容')).toBeNull()
    expect(screen.getByText('会话内容')).toBeInTheDocument()
  })

  it('页头操作由页面 portal 进来，且随页面切换（按钮仍属页面作用域）', async () => {
    const user = userEvent.setup()
    renderSecurity()

    const header = screen.getByRole('heading', { level: 1 }).closest('header') as HTMLElement
    expect(within(header).getByRole('button')).toHaveTextContent(zh.certs.title + zh.common.create)

    // 令牌页没有页头操作 → 按钮消失（说明它归页面，而不是布局的固定件）
    await user.click(screen.getByRole('link', { name: zh.nav.tokens }))
    expect(within(header).queryByRole('button')).toBeNull()
  })

  it('系统设置组：菜单与页头同样来自二级页清单', () => {
    const { container } = renderSettings()

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(zh.storage.title)
    expect(screen.getByText(zh.storage.subtitle)).toBeInTheDocument()
    expect(screen.getAllByRole('link')).toHaveLength(3)
    expect(container.querySelectorAll('.sliding-pill-indicator')).toHaveLength(1)
    expect(screen.getByRole('link', { name: zh.storage.title })).toHaveAttribute('data-active', 'true')
    expect(screen.getByRole('link', { name: zh.nav.asyncd })).toHaveAttribute('data-active', 'false')
  })

  it('系统设置是纵向菜单：分组抬头 + 行首图标，滑块改为沿 Y 轴滑动且导航常驻', async () => {
    const restore = stubLayout()
    try {
      const user = userEvent.setup()
      const { container } = renderSettings()

      // 纵向排列 + 分组抬头 + 每行一个图标（对齐组织管理的资源侧栏）
      const nav = container.querySelector('nav') as HTMLElement
      expect(nav.className).toContain('flex-col')
      expect(screen.getByText(zh.nav.settings)).toBeInTheDocument()
      for (const link of screen.getAllByRole('link')) {
        expect(link.querySelector('svg')).not.toBeNull()
      }

      const before = container.querySelector('.sliding-pill-indicator') as HTMLElement
      expect(before.style.transform).toBe('translate(4px, 4px)')

      await user.click(screen.getByRole('link', { name: zh.nav.asyncd }))

      const after = container.querySelector('.sliding-pill-indicator') as HTMLElement
      // 菜单由布局常驻 → 切换路由时是同一个 DOM 节点，位移过渡才有起点
      expect(after).toBe(before)
      expect(after.style.transform).toBe('translate(4px, 40px)')
      // 页头与二级页标题一起换
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(zh.asyncd.title)
    } finally {
      restore()
    }
  })

  it('安全管理仍是横向胶囊行（两种形态各自独立）', () => {
    const { container } = renderSecurity()
    const nav = container.querySelector('nav') as HTMLElement
    expect(nav.className).not.toContain('flex-col')
    // 侧栏槽位没有菜单：只有标签行，没有分组抬头
    expect(screen.queryByText(zh.nav.settings)).toBeNull()
  })
})
