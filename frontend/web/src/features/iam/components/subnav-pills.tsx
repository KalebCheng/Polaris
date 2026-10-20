import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { NavPills } from '@/widgets/layout/nav-pills'
import type { SubnavPageTab } from '@/widgets/layout/nav-config'

interface SubnavPillsProps {
  /** 导航区无障碍名称（i18n key，如 `nav.security` / `nav.settings`） */
  ariaLabelKey: string
  /** 二级页清单（`securityTabs` / `settingsTabs`，唯一声明处：nav-config） */
  tabs: SubnavPageTab[]
}

/**
 * 页面级二级标签（`PageShell` 的 `subnav` 槽位）：安全管理（证书 / 令牌 / 会话）与
 * 系统设置（存储设置 / 任务中心 / 审计日志）共用这一份实现。
 *
 * 选中态用的是顶栏导航同一块**滑动墨色胶囊**（`NavPills` → `shared/ui` 的 `useSlidingPill`）：
 * 二级只比一级低半级，不该是"另一套选中态语言"（下划线 / 主色填充 / 每个按钮自己点亮）——
 * 安全管理那份曾是手写的第三实现，结果**唯独它没有滑块**，看着像另一个控件。
 *
 * 容器样式（浅灰胶囊底 + 内边距）也只在这里写一次：两处各写一遍必然漂移。
 */
export function SubnavPills({ ariaLabelKey, tabs }: SubnavPillsProps) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  return (
    <div className="bg-muted/70 inline-flex items-center rounded-full p-1">
      <NavPills
        ariaLabel={t(ariaLabelKey)}
        itemClassName="h-8"
        items={tabs.map((tab) => ({ to: tab.to, label: t(tab.labelKey), active: tab.isActive(pathname) }))}
      />
    </div>
  )
}
