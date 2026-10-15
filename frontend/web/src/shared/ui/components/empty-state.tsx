import type { LucideIcon } from 'lucide-react'
import { Inbox } from 'lucide-react'
import { cn } from '@/shared/lib/utils'

interface EmptyStateProps {
  icon?: LucideIcon
  title: string
  description?: string
  action?: React.ReactNode
  className?: string
}

/** 空状态：列表与详情页统一使用，避免各页自行拼装 */
export function EmptyState({ icon: Icon = Inbox, title, description, action, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center gap-3.5 px-6 py-16 text-center', className)}>
      <div className="bg-primary/8 text-primary grid size-12 place-items-center rounded-2xl">
        <Icon className="size-5" />
      </div>
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        {description ? <p className="text-muted-foreground text-[13px]">{description}</p> : null}
      </div>
      {action}
    </div>
  )
}
