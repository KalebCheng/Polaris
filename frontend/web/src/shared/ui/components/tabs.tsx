import { createContext, useContext, useRef } from 'react'
import * as TabsPrimitive from '@radix-ui/react-tabs'
import { cn } from '@/shared/lib/utils'
import { SlidingPill, useSlidingPill } from './sliding-pill'

/** 分段控件形态：`pill`＝胶囊分段（滑块表达选中态）；`plain`＝自由样式（如登录页的下划线标签） */
type TabsVariant = 'pill' | 'plain'
/** `TabsList` 声明形态，`TabsTrigger` 据此决定选中态怎么画 */
const TabsVariantContext = createContext<TabsVariant>('pill')

/**
 * 分段控件（受控 / 非受控都直接透传给 Radix）。
 * 滑块的量测不再依赖"选中值变化"这条信号（量测每次提交都做，见 `useSlidingPill`），
 * 因此这里不再需要把当前值沿组件树传给 `TabsList`。
 */
export const Tabs = TabsPrimitive.Root

export interface TabsListProps extends React.ComponentProps<typeof TabsPrimitive.List> {
  /**
   * `pill`（默认）＝胶囊分段控件，选中态是**一块滑动的墨色胶囊**（与顶栏导航同一套滑块）；
   * `plain` ＝不带滑块，样式完全由调用方决定（登录页的下划线标签）。
   */
  variant?: TabsVariant
}

export function TabsList({ className, variant = 'pill', children, ...props }: TabsListProps) {
  const listRef = useRef<HTMLDivElement>(null)
  // 量测必须在**容器组件**里做：子组件的 layout effect 早于容器 ref 挂载（见 sliding-pill.tsx）
  const { box, ready } = useSlidingPill(listRef, {
    activeSelector: '[data-state="active"]',
    enabled: variant === 'pill',
  })

  return (
    <TabsPrimitive.List
      ref={listRef}
      data-slot="tabs-list"
      data-variant={variant}
      className={cn(
        'bg-muted/70 text-muted-foreground inline-flex h-10 items-center justify-center gap-1 rounded-full p-1',
        variant === 'pill' && 'relative',
        className,
      )}
      {...props}
    >
      {variant === 'pill' ? <SlidingPill box={box} ready={ready} /> : null}
      <TabsVariantContext.Provider value={variant}>{children}</TabsVariantContext.Provider>
    </TabsPrimitive.List>
  )
}

/**
 * 分段控件项。
 *
 * 选中项**只改文字色**，底色交给滑块（`SlidingPill`）——否则会出现"一块动的 + 一块不动的"
 * 重影（与 `nav-pills.tsx` 同一条约定，两个测试文件都锁定）。
 * `plain` 形态下选中态完全由调用方的 className 决定。
 */
export function TabsTrigger({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Trigger>) {
  const variant = useContext(TabsVariantContext)
  return (
    <TabsPrimitive.Trigger
      data-slot="tabs-trigger"
      className={cn(
        'inline-flex flex-1 items-center justify-center gap-1.5 rounded-full px-3.5 py-1.5 text-[13px] font-medium whitespace-nowrap transition-colors',
        'focus-visible:ring-ring/25 focus-visible:ring-4 disabled:pointer-events-none disabled:opacity-50',
        'hover:text-foreground',
        // 压在滑块之上：滑块是 z-0 的绝对定位元素，静态定位的同级元素会被它盖住
        variant === 'pill' && 'relative z-10 data-[state=active]:text-ink-foreground',
        '[&_svg]:size-4 [&_svg]:shrink-0',
        className,
      )}
      {...props}
    />
  )
}

export function TabsContent({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Content>) {
  return (
    <TabsPrimitive.Content
      data-slot="tabs-content"
      className={cn('flex-1 outline-none', className)}
      {...props}
    />
  )
}
