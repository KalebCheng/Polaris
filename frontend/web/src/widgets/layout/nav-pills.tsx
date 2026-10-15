import { useRef } from 'react'
import { NavLink } from 'react-router-dom'
import { ChevronDown, type LucideIcon } from 'lucide-react'
import { SlidingPill, useSlidingPill } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

/** 可跳转的条目 */
export interface NavPillItem {
  kind?: 'link'
  to: string
  label: string
  active: boolean
  /** 纵向菜单里的行首图标（横向胶囊行不带图标） */
  icon?: LucideIcon
  /** 精确匹配：`/institutions/3` 这类"前缀会命中子页"的条目必须开 */
  end?: boolean
  /** 禁用（例如尚未选择组织）：只改可交互性，不参与选中态 */
  disabled?: boolean
}

/**
 * 分组抬头（不可跳转、不参与选中态）。
 *
 * 为什么抬头要能进同一个列表：滑块是"在**容器内**量测选中项"，
 * 一个菜单只有一块容器时，跨分组切换才能是同一块滑块的位移；
 * 若每个分组各渲染一份列表，跨分组就是"旧滑块卸载 + 新滑块就位"——没有过渡。
 */
export interface NavPillSection {
  kind: 'section'
  label: string
  expanded: boolean
  onToggle: () => void
}

export type NavEntry = NavPillItem | NavPillSection

interface NavPillsProps {
  items: NavEntry[]
  ariaLabel: string
  /** 容器额外 class（例如窄屏下 `overflow-x-auto` 横滑、`hidden md:flex` 显隐） */
  className?: string
  /** 单项额外 class（改高度 / 内边距档位） */
  itemClassName?: string
  /**
   * 排列方向：横向＝顶栏/页头那排胶囊；纵向＝侧栏式菜单（行满宽、行首图标、圆角改为方块）。
   * 两种方向共用同一块滑块，只是位移轴与圆角不同——选中态语言因此始终只有一套。
   */
  orientation?: 'horizontal' | 'vertical'
  /** 点击后回调（窄屏抽屉里用来关抽屉） */
  onNavigate?: () => void
}

/**
 * 顶栏导航（胶囊行）——选中态是**一块会滑过去的墨色胶囊**，切换时用弹性缓动跟手移动。
 *
 * 滑块本体（量测 / 首帧关过渡 / ResizeObserver / 降级）由 `shared/ui` 的 `SlidingPill` 提供，
 * 分段控件 `Tabs` 用的是同一份实现——两处的位移手感与令牌因此不会各自漂移。
 */
export function NavPills({
  items,
  ariaLabel,
  className,
  itemClassName,
  orientation = 'horizontal',
  onNavigate,
}: NavPillsProps) {
  const listRef = useRef<HTMLDivElement>(null)
  const vertical = orientation === 'vertical'
  const hasActive = items.some((item) => item.kind !== 'section' && item.active)
  // 量测由 hook 在每次提交后自行完成（含折叠、增删条目、换语言）
  const { box, ready } = useSlidingPill(listRef, { activeSelector: '[data-active="true"]' })

  return (
    <nav
      ref={listRef}
      aria-label={ariaLabel}
      className={cn('relative flex gap-1', vertical ? 'flex-col' : 'items-center', className)}
    >
      {/* 纵向时滑块跟着行一起拉满宽高，圆角收成与行一致（胶囊是横向的形态语言） */}
      {hasActive ? <SlidingPill box={box} ready={ready} className={vertical ? 'rounded-xl' : undefined} /> : null}
      {items.map((item, index) =>
        item.kind === 'section' ? (
          <button
            key={`section:${item.label}`}
            type="button"
            aria-expanded={item.expanded}
            onClick={item.onToggle}
            className={cn(
              'text-muted-foreground hover:text-foreground flex items-center justify-between px-3.5 py-1 text-[10px] font-semibold tracking-wide transition-colors',
              // 纵向菜单里分组之间留出节奏（单容器渲染后 gap 靠这里补）
              vertical && index > 0 && 'mt-2',
            )}
          >
            {item.label}
            <ChevronDown className={cn('size-3.5 transition-transform', !item.expanded && '-rotate-90')} />
          </button>
        ) : (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.end}
          aria-disabled={item.disabled || undefined}
          tabIndex={item.disabled ? -1 : undefined}
          onClick={onNavigate}
          data-active={item.active}
          className={cn(
            vertical
              ? 'relative z-10 flex w-full items-center gap-2.5 rounded-xl px-3.5 py-2 text-[13px] font-medium transition-colors'
              : 'relative z-10 inline-flex h-9 shrink-0 items-center rounded-full px-3.5 text-[13px] font-medium whitespace-nowrap transition-colors',
            item.active
              ? 'text-ink-foreground'
              : 'text-muted-foreground hover:bg-accent hover:text-foreground',
            item.disabled && 'pointer-events-none opacity-40',
            itemClassName,
          )}
        >
          {item.icon ? <item.icon className="size-4 shrink-0" /> : null}
          <span className="truncate">{item.label}</span>
        </NavLink>
        ),
      )}
    </nav>
  )
}
