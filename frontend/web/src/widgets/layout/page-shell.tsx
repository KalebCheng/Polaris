import { useTranslation } from 'react-i18next'
import { cn } from '@/shared/lib/utils'

interface PageShellProps {
  /** i18n key：页面标题 */
  titleKey?: string
  /** i18n key：页面副标题 */
  subtitleKey?: string
  /** 直接给出的标题（优先于 titleKey，用于含动态内容的场景） */
  title?: React.ReactNode
  /** 直接给出的描述 */
  description?: React.ReactNode
  actions?: React.ReactNode
  /** 二级标签区（如安全管理下的证书 / 令牌 / 会话） */
  subnav?: React.ReactNode
  children: React.ReactNode
  className?: string
}

/**
 * 页面骨架：标题 + 描述 + 操作区，可选二级标签。
 *
 * 标题字阶对齐参考视觉（约 26px、半粗、负字距），描述压到 13px 的次级灰——
 * 一屏里"这是什么页面"必须比"这页有什么字段"更先被读到。
 * 内容区贴边铺满，仅保留必要的呼吸间距。
 */
export function PageShell({
  titleKey,
  subtitleKey,
  title,
  description,
  actions,
  subnav,
  children,
  className,
}: PageShellProps) {
  const { t } = useTranslation()
  const heading = title ?? (titleKey ? t(titleKey) : null)
  const sub = description ?? (subtitleKey ? t(subtitleKey) : null)

  return (
    <div className={cn('flex flex-col', className)}>
      <header className="flex flex-col gap-3 pb-1 sm:flex-row sm:items-start sm:justify-between sm:gap-6">
        <div className="min-w-0 space-y-2">
          {heading ? (
            <h1 className="text-[26px] leading-tight font-semibold tracking-tight">{heading}</h1>
          ) : null}
          {sub ? (
            <p className="text-muted-foreground max-w-3xl text-[13px] leading-relaxed">{sub}</p>
          ) : null}
        </div>
        {actions ? (
          <div className="flex shrink-0 flex-wrap items-center gap-2 sm:pt-1">{actions}</div>
        ) : null}
      </header>

      {subnav ? <div className="mt-5">{subnav}</div> : null}

      <div className="mt-6 flex flex-col gap-5">{children}</div>
    </div>
  )
}
