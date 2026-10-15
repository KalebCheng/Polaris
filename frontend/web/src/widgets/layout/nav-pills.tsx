import { useRef } from 'react'
import { NavLink } from 'react-router-dom'
import { SlidingPill, useSlidingPill } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

export interface NavPillItem {
  to: string
  label: string
  active: boolean
}

interface NavPillsProps {
  items: NavPillItem[]
  ariaLabel: string
  /** 容器额外 class（例如窄屏下 `overflow-x-auto` 横滑、`hidden md:flex` 显隐） */
  className?: string
  /** 单项额外 class（改高度 / 内边距档位） */
  itemClassName?: string
  /** 点击后回调（窄屏抽屉里用来关抽屉） */
  onNavigate?: () => void
}

/**
 * 顶栏导航（胶囊行）——选中态是**一块会滑过去的墨色胶囊**，切换时用弹性缓动跟手移动。
 *
 * 滑块本体（量测 / 首帧关过渡 / ResizeObserver / 降级）由 `shared/ui` 的 `SlidingPill` 提供，
 * 分段控件 `Tabs` 用的是同一份实现——两处的位移手感与令牌因此不会各自漂移。
 */
export function NavPills({ items, ariaLabel, className, itemClassName, onNavigate }: NavPillsProps) {
  const listRef = useRef<HTMLDivElement>(null)
  const activeIndex = items.findIndex((item) => item.active)
  // 依赖签名：标签（切语言）、目标或选中项变化都要重新量一次
  const signature = items.map((item) => `${item.to}:${item.label}:${item.active ? 1 : 0}`).join('|')
  const { box, ready } = useSlidingPill(listRef, {
    activeSelector: '[data-active="true"]',
    measureKey: signature,
  })

  return (
    <nav ref={listRef} aria-label={ariaLabel} className={cn('relative flex items-center gap-1', className)}>
      {activeIndex >= 0 ? <SlidingPill box={box} ready={ready} /> : null}
      {items.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          onClick={onNavigate}
          data-active={item.active}
          className={cn(
            'relative z-10 inline-flex h-9 shrink-0 items-center rounded-full px-3.5 text-[13px] font-medium whitespace-nowrap transition-colors',
            item.active
              ? 'text-ink-foreground'
              : 'text-muted-foreground hover:bg-accent hover:text-foreground',
            itemClassName,
          )}
        >
          {item.label}
        </NavLink>
      ))}
    </nav>
  )
}
