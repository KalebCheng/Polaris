import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { NavPills } from '@/widgets/layout/nav-pills'
import { settingsTabs } from '@/widgets/layout/nav-config'

/**
 * 「设置」二级标签：存储设置 / 任务中心 / 审计日志。
 *
 * 用顶栏导航同一块**滑动墨色胶囊**（`NavPills` 复用 `shared/ui` 的 `useSlidingPill`）：
 * 二级导航只比一级低半级，不应该是"另外一套选中态语言"（下划线 / 主色填充）。
 */
export function SettingsSubnav() {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const items = settingsTabs.map((tab) => ({
    to: tab.to,
    label: t(tab.labelKey),
    active: tab.isActive(pathname),
  }))

  return (
    <div className="bg-muted/70 inline-flex items-center rounded-full p-1">
      <NavPills items={items} ariaLabel={t('nav.settings')} itemClassName="h-8" />
    </div>
  )
}
