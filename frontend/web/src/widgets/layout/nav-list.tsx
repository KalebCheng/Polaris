import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { useAuth } from '@/shared/auth/auth-context'
import { useCurrentInstitution } from '@/features/iam/institution-context'
import { NavMenu } from './nav-menu'
import { workspacePath, workspaceSections, type NavItem } from './nav-config'

interface WorkspaceNavProps {
  /** 点击任一导航项后的回调（移动端用于关闭抽屉） */
  onNavigate?: () => void
}

/** 工作区导航：分组可折叠的当前组织资源菜单 */
export function WorkspaceNav({ onNavigate }: WorkspaceNavProps) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const { can } = useAuth()
  const { institutionId } = useCurrentInstitution()
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})

  const visible = (item: NavItem) => (!item.resource ? true : can(item.resource, 'read'))
  const disabled = !institutionId

  /** 选中判定与 `NavLink` 同口径（`end` 用于总览这类"前缀会命中子页"的条目） */
  const isActive = (to: string, end: boolean) =>
    end ? pathname === to : pathname === to || pathname.startsWith(`${to}/`)

  /** 分组 → 菜单条目（含抬头）：整块菜单交给一个 `NavMenu`，跨分组切换才是同一块滑块的位移 */
  const sections = workspaceSections
    .map((section) => ({ section, items: section.items.filter(visible) }))
    .filter((entry) => entry.items.length > 0)
    .map(({ section, items }) => ({
      labelKey: section.labelKey,
      collapsed: Boolean(collapsed[section.labelKey]),
      onToggle: () =>
        setCollapsed((current) => ({ ...current, [section.labelKey]: !current[section.labelKey] })),
      items: items.map((item) => {
        const to = workspacePath(institutionId, item.to)
        const end = item.to === ''
        return {
          to,
          labelKey: item.labelKey,
          icon: item.icon,
          end,
          disabled,
          // 未选组织时整列都不可用，此时不给任何条目上选中态
          active: !disabled && isActive(to, end),
        }
      }),
    }))

  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-2.5 pt-1 pb-4">
      {institutionId ? (
        <NavMenu sections={sections} ariaLabel={t('nav.workspaceNav')} onNavigate={onNavigate} />
      ) : (
        <p className="text-muted-foreground px-3.5 py-2 text-xs">{t('institutions.selectHint')}</p>
      )}
    </div>
  )
}
