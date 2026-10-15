import { useTranslation } from 'react-i18next'

interface PortalPageProps {
  titleKey: string
  subtitleKey?: string
  actions?: React.ReactNode
  children: React.ReactNode
}

/**
 * 门户页面骨架：与控制台 PageShell 同一套排版（标题 + 描述 + 操作区），
 * 内容铺满、仅保留必要间距。
 */
export function PortalPage({ titleKey, subtitleKey, actions, children }: PortalPageProps) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1.5">
          <h1 className="text-[19px] font-semibold tracking-tight">{t(titleKey)}</h1>
          {subtitleKey ? <p className="text-muted-foreground max-w-2xl text-xs leading-relaxed">{t(subtitleKey)}</p> : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
      </header>
      <div className="mt-5 flex flex-col gap-5">{children}</div>
    </div>
  )
}
