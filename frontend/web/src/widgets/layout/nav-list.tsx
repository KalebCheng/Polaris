import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { NavLink } from 'react-router-dom'
import { ChevronDown } from 'lucide-react'
import { cn } from '@/shared/lib/utils'
import { useAuth } from '@/features/auth/auth-context'
import { useCurrentInstitution } from '@/features/iam/institution-context'
import { workspacePath, workspaceSections, type NavItem, type NavSection } from './nav-config'

interface WorkspaceNavProps {
  /** 点击任一导航项后的回调（移动端用于关闭抽屉） */
  onNavigate?: () => void
}

/** 工作区导航：分组可折叠的当前组织资源菜单 */
export function WorkspaceNav({ onNavigate }: WorkspaceNavProps) {
  const { t } = useTranslation()
  const { can } = useAuth()
  const { institutionId } = useCurrentInstitution()
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})

  const visible = (item: NavItem) => (!item.resource ? true : can(item.resource, 'read'))
  const disabled = !institutionId

  const toggle = (labelKey: string) =>
    setCollapsed((current) => ({ ...current, [labelKey]: !current[labelKey] }))

  const renderSection = (section: NavSection) => {
    const items = section.items.filter(visible)
    if (items.length === 0) return null
    const isCollapsed = collapsed[section.labelKey]
    return (
      <div key={section.labelKey} className="flex flex-col gap-1">
        <button
          type="button"
          aria-expanded={!isCollapsed}
          onClick={() => toggle(section.labelKey)}
          className="text-muted-foreground hover:text-foreground flex items-center justify-between px-3.5 py-1 text-[10px] font-semibold tracking-wide transition-colors"
        >
          {t(section.labelKey)}
          <ChevronDown className={cn('size-3.5 transition-transform', isCollapsed && '-rotate-90')} />
        </button>
        {!isCollapsed ? (
          <div className="flex flex-col gap-1">
            {items.map((item) => (
              <NavLink
                key={item.to || 'overview'}
                to={workspacePath(institutionId, item.to)}
                end={item.to === ''}
                onClick={onNavigate}
                aria-disabled={disabled}
                tabIndex={disabled ? -1 : undefined}
                className={({ isActive }) =>
                  cn(
                    'flex items-center gap-2.5 rounded-xl px-3.5 py-2 text-[13px] transition-colors',
                    disabled && 'pointer-events-none opacity-40',
                    isActive && !disabled
                      ? 'bg-primary/10 text-primary font-medium'
                      : 'text-muted-foreground hover:bg-accent hover:text-foreground',
                  )
                }
              >
                <item.icon className="size-4 shrink-0" />
                <span className="truncate">{t(item.labelKey)}</span>
              </NavLink>
            ))}
          </div>
        ) : null}
      </div>
    )
  }

  return (
    <nav className="flex flex-1 flex-col gap-3 overflow-y-auto px-2.5 pt-2 pb-4" aria-label={t('nav.workspaceNav')}>
      {institutionId ? (
        workspaceSections.map(renderSection)
      ) : (
        <p className="text-muted-foreground px-3.5 py-2 text-xs">{t('institutions.selectHint')}</p>
      )}
    </nav>
  )
}
