import { useTranslation } from 'react-i18next'
import { NavLink } from 'react-router-dom'
import { cn } from '@/shared/lib/utils'
import { securityTabs } from '@/widgets/layout/nav-config'

/** 安全管理二级标签：证书 / 令牌 / 会话（与页内分段控件同一形态） */
export function SecuritySubnav() {
  const { t } = useTranslation()
  return (
    <div
      className="bg-muted/70 inline-flex items-center gap-1 rounded-full p-1"
      role="tablist"
      aria-label={t('nav.security')}
    >
      {securityTabs.map((tab) => (
        <NavLink
          key={tab.to}
          to={tab.to}
          role="tab"
          className={({ isActive }) =>
            cn(
              'inline-flex h-8 items-center rounded-full px-3.5 text-[13px] font-medium transition-colors',
              isActive ? 'bg-ink text-ink-foreground' : 'text-muted-foreground hover:text-foreground',
            )
          }
        >
          {t(tab.labelKey)}
        </NavLink>
      ))}
    </div>
  )
}
