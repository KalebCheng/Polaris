import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown } from 'lucide-react'
import { cn } from '@/shared/lib/utils'

interface FormSectionProps {
  /**
   * i18n key：分区标题。
   * 省略时不渲染标题区——用于"向导头部已经承载了本步标题"的场景，
   * 避免同一个标题在一屏里出现两次（此时分区只作为字段容器/卡片）。
   */
  titleKey?: string
  /** i18n key：分区说明（一句话讲清这一组字段的作用） */
  descriptionKey?: string
  /** 次要 / 高级字段：默认折叠，点击标题展开 */
  collapsible?: boolean
  /** 折叠分区的初始展开状态（默认折叠） */
  defaultOpen?: boolean
  children: React.ReactNode
  className?: string
}

/**
 * 表单分区：一个语义分组（标题 + 说明 + 字段）。
 * 统一「新建 / 编辑」页的信息层级——用分区把成组的字段收拢，而不是把几十个字段平铺成一堵墙。
 * 高级字段用 `collapsible` 收起，首屏只暴露常用项，降低认知负担。
 */
export function FormSection({
  titleKey,
  descriptionKey,
  collapsible = false,
  defaultOpen = false,
  children,
  className,
}: FormSectionProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(collapsible ? defaultOpen : true)
  const expanded = collapsible ? open : true

  const header = (
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0 space-y-0.5">
        {titleKey ? <h3 className="text-[13px] font-semibold tracking-tight">{t(titleKey)}</h3> : null}
        {descriptionKey ? (
          <p className="text-muted-foreground text-xs leading-relaxed">{t(descriptionKey)}</p>
        ) : null}
      </div>
      {collapsible ? (
        <ChevronDown
          className={cn(
            'text-muted-foreground mt-0.5 size-4 shrink-0 transition-transform duration-200',
            !expanded && '-rotate-90',
          )}
        />
      ) : null}
    </div>
  )

  const hasHeader = Boolean(titleKey)

  return (
    <section className={cn('bg-card border-border/70 shadow-soft overflow-hidden rounded-2xl border', className)}>
      {hasHeader ? (
        collapsible ? (
          <button
            type="button"
            className="hover:bg-accent/40 w-full px-5 py-3.5 text-left transition-colors"
            aria-expanded={expanded}
            onClick={() => setOpen((value) => !value)}
          >
            {header}
          </button>
        ) : (
          <div className="px-5 py-3.5">{header}</div>
        )
      ) : null}
      {expanded ? <div className={cn('px-5 py-4', hasHeader && 'border-t')}>{children}</div> : null}
    </section>
  )
}
