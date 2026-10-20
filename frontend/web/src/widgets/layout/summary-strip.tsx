import { Link } from 'react-router-dom'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/shared/lib/utils'

export interface SummaryItem {
  label: string
  value: React.ReactNode
  /** 值的语气：success 用青绿，warning 用琥珀，danger 用红 */
  tone?: 'default' | 'success' | 'warning' | 'danger'
  /** 可选前置图标（只在 KPI 卡片形态下渲染） */
  icon?: LucideIcon
  /** 可选补充说明（口径 / 时间范围 / 数据来源） */
  hint?: string
  /** 可选跳转地址：给出时整张卡片可点（KPI 卡片行即"入口"） */
  to?: string
}

interface SummaryStripProps {
  items: SummaryItem[]
  /**
   * `cards`（默认）＝页面顶部的 KPI 卡片行（参考视觉：图标方块 + 大数字 + 口径）；
   * `inline` ＝紧凑一行，用于弹窗、卡片内部的次级统计（放四个大白卡会把容器撑爆）。
   */
  variant?: 'cards' | 'inline'
  className?: string
}

const toneClass: Record<NonNullable<SummaryItem['tone']>, string> = {
  default: 'text-foreground',
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-destructive',
}

/**
 * 概况条 / KPI 行：列表页顶部统一"对齐口径"的位置。
 * 默认渲染为**卡片行**——数值要大到一眼可读，标签要小到不抢戏（参考视觉的核心对比关系）。
 */
export function SummaryStrip({ items, variant = 'cards', className }: SummaryStripProps) {
  if (items.length === 0) return null

  if (variant === 'inline') {
    return (
      <div
        className={cn(
          'bg-muted/40 border-border/70 flex flex-wrap items-center gap-x-6 gap-y-2 rounded-xl border px-4 py-2.5',
          className,
        )}
      >
        {items.map((item, index) => (
          <div key={index} className="flex items-baseline gap-2">
            <span className="text-muted-foreground text-[11px]">{item.label}</span>
            <span className={cn('text-[13px] font-semibold tabular-nums', toneClass[item.tone ?? 'default'])}>
              {item.value}
            </span>
          </div>
        ))}
      </div>
    )
  }

  return (
    <div className={cn('grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4', className)}>
      {items.map((item, index) => (
        <StatCard key={index} item={item} />
      ))}
    </div>
  )
}

function StatCard({ item }: { item: SummaryItem }) {
  const body = (
    <>
      <div className="flex items-center gap-2">
        {item.icon ? (
          <span className="bg-primary/10 text-primary grid size-8 shrink-0 place-items-center rounded-[10px]">
            <item.icon className="size-4" />
          </span>
        ) : null}
        <span className="text-muted-foreground truncate text-[11px] font-medium">{item.label}</span>
      </div>
      <span
        className={cn(
          'leading-none font-semibold tracking-tight tabular-nums',
          // 数值用大字号；口径类文本值（如"全平台"）压小一档，否则一句话被放大成"大标题"
          typeof item.value === 'string' ? 'text-[17px]' : 'text-[22px]',
          toneClass[item.tone ?? 'default'],
        )}
      >
        {item.value}
      </span>
      {item.hint ? <span className="text-muted-foreground text-[11px]">{item.hint}</span> : null}
    </>
  )

  const cardClass = cn(
    'surface flex flex-col gap-2.5 rounded-2xl px-4 py-3.5',
    item.to && 'hover:border-primary/30 hover:shadow-lifted transition-colors',
  )

  if (item.to) {
    return (
      <Link to={item.to} className={cardClass}>
        {body}
      </Link>
    )
  }
  return <div className={cardClass}>{body}</div>
}
