import { cn } from '@/shared/lib/utils'

export type StatusTone = 'success' | 'warning' | 'danger' | 'info' | 'neutral'

const toneClass: Record<StatusTone, string> = {
  success: 'bg-success/12 text-success border-success/15',
  warning: 'bg-warning/14 text-warning border-warning/20',
  danger: 'bg-destructive/10 text-destructive border-destructive/15',
  info: 'bg-info/10 text-info border-info/15',
  neutral: 'bg-muted/70 text-muted-foreground border-border/70',
}

interface StatusPillProps {
  tone?: StatusTone
  children: React.ReactNode
  /** 是否显示前置圆点 */
  dot?: boolean
  className?: string
}

/** 状态药丸：柔和底色的「状态」标签，列表与概况条统一使用 */
export function StatusPill({ tone = 'neutral', children, dot = true, className }: StatusPillProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[11px] font-medium whitespace-nowrap',
        toneClass[tone],
        className,
      )}
    >
      {dot ? <span className="size-1.5 shrink-0 rounded-full bg-current" /> : null}
      {children}
    </span>
  )
}
