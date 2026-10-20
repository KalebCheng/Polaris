import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/ui'

const box = (left: number, top: number, width: number, height: number) =>
  ({ left, top, width, height, right: left + width, bottom: top + height, x: left, y: top }) as DOMRect

function PillTabs({ variant }: { variant?: 'pill' | 'plain' }) {
  return (
    <Tabs defaultValue="in">
      <TabsList variant={variant}>
        <TabsTrigger value="in">入方向</TabsTrigger>
        <TabsTrigger value="out">出方向</TabsTrigger>
      </TabsList>
      <TabsContent value="in">入方向内容</TabsContent>
      <TabsContent value="out">出方向内容</TabsContent>
    </Tabs>
  )
}

function ControlledTabs() {
  const [value, setValue] = useState('in')
  return (
    <Tabs value={value} onValueChange={setValue}>
      <TabsList>
        <TabsTrigger value="in">入方向</TabsTrigger>
        <TabsTrigger value="out">出方向</TabsTrigger>
      </TabsList>
    </Tabs>
  )
}

/**
 * 给 jsdom 装上"有尺寸"的矩形（默认全 0，滑块位置恒为 0，锁不住"会不会跟着动"）。
 * 刻意把容器放在 (40, 100)：滑块用的是**两者 rect 之差**，所以期望值是 4px 而不是绝对坐标——
 * 这条口径是"窄屏横向滚动 / 容器不在页面左上角时也算得对"的前提。
 */
function stubLayout() {
  const original = Element.prototype.getBoundingClientRect
  Element.prototype.getBoundingClientRect = function (this: HTMLElement) {
    if ((this as HTMLElement).dataset.slot === 'tabs-list') return box(40, 100, 200, 40)
    const label = this.textContent ?? ''
    if (label.includes('入方向')) return box(44, 104, 100, 32)
    if (label.includes('出方向')) return box(144, 104, 100, 32)
    return box(0, 0, 0, 0)
  }
  return () => {
    Element.prototype.getBoundingClientRect = original
  }
}

describe('Tabs 分段控件滑块', () => {
  it('默认渲染唯一一块滑块，且对读屏隐藏', () => {
    const { container } = render(<PillTabs />)

    const indicators = container.querySelectorAll('.sliding-pill-indicator')
    expect(indicators).toHaveLength(1)
    expect(indicators[0]).toHaveAttribute('aria-hidden', 'true')
  })

  it('选中项只改文字色，不自己画墨色底（底色由滑块负责）', () => {
    const { container } = render(<PillTabs />)

    const active = screen.getByRole('tab', { name: '入方向' })
    expect(active).toHaveAttribute('data-state', 'active')
    expect(active.className).toContain('data-[state=active]:text-ink-foreground')

    // 底色只允许来自滑块：任何一项都不带 bg-ink（否则会出现"一块动的 + 一块不动的"重影）
    for (const tab of screen.getAllByRole('tab')) {
      expect(tab.className).not.toContain('bg-ink')
    }
    expect(container.querySelectorAll('.sliding-pill-indicator')).toHaveLength(1)
  })

  it('切换后选中态转移，滑块仍只有一块（量测随选中值重算）', async () => {
    const user = userEvent.setup()
    const { container } = render(<PillTabs />)

    await user.click(screen.getByRole('tab', { name: '出方向' }))

    expect(screen.getByRole('tab', { name: '出方向' })).toHaveAttribute('data-state', 'active')
    expect(screen.getByRole('tab', { name: '入方向' })).toHaveAttribute('data-state', 'inactive')
    expect(container.querySelectorAll('.sliding-pill-indicator')).toHaveLength(1)
    // 内容也跟着切换（确认包装 Tabs 没有破坏 Radix 的基本行为）
    expect(screen.getByText('出方向内容')).toBeInTheDocument()
  })

  it('包装后受控用法仍然生效（value 由外部决定）', async () => {
    const user = userEvent.setup()
    render(<ControlledTabs />)

    await user.click(screen.getByRole('tab', { name: '出方向' }))
    expect(screen.getByRole('tab', { name: '出方向' })).toHaveAttribute('data-state', 'active')
    expect(screen.getByRole('tab', { name: '入方向' })).toHaveAttribute('data-state', 'inactive')
  })

  it('选中值不存在时（没有 active 项）不渲染滑块', () => {
    const { container } = render(
      <Tabs value="missing">
        <TabsList>
          <TabsTrigger value="in">入方向</TabsTrigger>
          <TabsTrigger value="out">出方向</TabsTrigger>
        </TabsList>
      </Tabs>,
    )

    expect(container.querySelectorAll('.sliding-pill-indicator')).toHaveLength(0)
  })

  it('切换后滑块位置跟着重算（弹性过渡的前提）', async () => {
    const restore = stubLayout()
    try {
      const user = userEvent.setup()
      const { container } = render(<PillTabs />)
      const transform = () =>
        (container.querySelector('.sliding-pill-indicator') as HTMLElement).style.transform

      expect(transform()).toBe('translate(4px, 4px)')
      await user.click(screen.getByRole('tab', { name: '出方向' }))
      expect(transform()).toBe('translate(104px, 4px)')
    } finally {
      restore()
    }
  })

  it('variant="plain" 不套滑块，选中态交回调用方（登录页的下划线标签）', () => {
    const { container } = render(<PillTabs variant="plain" />)

    expect(container.querySelectorAll('.sliding-pill-indicator')).toHaveLength(0)
    // plain 下不输出 pill 的文字色，也不给滑块让位（无 relative z-10）
    const active = screen.getByRole('tab', { name: '入方向' })
    expect(active.className).not.toContain('text-ink-foreground')
    expect(active.className).not.toContain('z-10')
  })
})
