import { useEffect, useLayoutEffect, useState } from 'react'
import { cn } from '@/shared/lib/utils'

export interface SlidingPillBox {
  left: number
  top: number
  width: number
  height: number
}

export interface SlidingPillOptions {
  /** 选中项选择器：导航用 `[data-active="true"]`，分段控件用 `[data-state="active"]` */
  activeSelector: string
  /** 量测依赖签名：选中项、标签文案变化时必须改变，否则不会重新量测 */
  measureKey: string
  /** 关掉量测（如 `plain` 形态的分段控件），返回的 box 恒为 null */
  enabled?: boolean
}

/** 选中项相对容器的位置：两者 rect 之差 + 滚动偏移 */
function measureActive(container: HTMLElement, selector: string): SlidingPillBox | null {
  const active = container.querySelector<HTMLElement>(selector)
  if (!active) return null
  const containerRect = container.getBoundingClientRect()
  const activeRect = active.getBoundingClientRect()
  return {
    left: activeRect.left - containerRect.left + container.scrollLeft,
    top: activeRect.top - containerRect.top + container.scrollTop,
    width: activeRect.width,
    height: activeRect.height,
  }
}

/**
 * 量测"选中项在哪"：**必须由容器组件自己调用**，不能放进 `SlidingPill` 里。
 *
 * 原因是 React 的提交时序：layout 阶段先递归子树、再给自己的 host 节点挂 ref ——
 * 也就是说子组件的 `useLayoutEffect` 里读 `containerRef.current` 一定是 `null`
 * （本文件第一版就是这么写的：滑块一直不出现，因为量测直接早退）。
 *
 * 因此职责切成两半：**量测在容器（本 hook）**，滑块只负责画。
 */
export function useSlidingPill(
  containerRef: React.RefObject<HTMLElement | null>,
  { activeSelector, measureKey, enabled = true }: SlidingPillOptions,
) {
  const [box, setBox] = useState<SlidingPillBox | null>(null)
  const [ready, setReady] = useState(false)

  useLayoutEffect(() => {
    if (!enabled) {
      setBox(null)
      return
    }
    const container = containerRef.current
    if (!container) return
    const measure = () => {
      const next = measureActive(container, activeSelector)
      // 值相同就返回旧引用，避免"每次渲染都 setState → 再渲染"的死循环
      setBox((prev) =>
        prev &&
        next &&
        prev.left === next.left &&
        prev.top === next.top &&
        prev.width === next.width &&
        prev.height === next.height
          ? prev
          : next,
      )
    }
    measure()
    // 容器尺寸变化（切语言、字体加载完、窄屏换行）都要重新量
    const observer = new ResizeObserver(measure)
    observer.observe(container)
    return () => observer.disconnect()
  }, [containerRef, activeSelector, measureKey, enabled])

  // 首帧关过渡：否则页面加载时滑块会从最左侧"飞"过去
  useEffect(() => {
    const frame = requestAnimationFrame(() => setReady(true))
    return () => cancelAnimationFrame(frame)
  }, [])

  return { box, ready }
}

/**
 * 弹性滑块：选中项背后那块"会滑过去"的墨色胶囊——导航（`NavPills`）与分段控件（`Tabs`）
 * 共用同一份量测与样式，因此位移手感、时长与降级口径只有一处定义。
 *
 * 为什么不做"每个按钮自己点亮"：切换时是一块实体在空间里移动，用户能顺着运动方向看到
 * "我从哪来、到哪去"；瞬时变色丢掉了这条信息，也没有手感。缓动与时长是令牌
 * （`--motion-spring` 等，见 `globals.css` 的 `.sliding-pill-indicator`），组件里不写死数值。
 *
 * 兜底口径（与量测同源）：找不到选中项（box 为 null）时不渲染；`prefers-reduced-motion`
 * 下由 CSS 关闭过渡。
 */
export function SlidingPill({
  box,
  ready,
  className,
}: {
  box: SlidingPillBox | null
  ready: boolean
  className?: string
}) {
  if (!box) return null
  return (
    <span
      aria-hidden="true"
      data-ready={ready}
      className={cn(
        'sliding-pill-indicator bg-ink shadow-soft pointer-events-none absolute top-0 left-0 z-0 rounded-full',
        className,
      )}
      style={{
        transform: `translate(${box.left}px, ${box.top}px)`,
        width: box.width,
        height: box.height,
      }}
    />
  )
}
