import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Outlet, useLocation } from 'react-router-dom'
import { cn } from '@/shared/lib/utils'
import { PageShell } from '@/widgets/layout/page-shell'
import { NAV_PANEL_CLASS, NavMenu } from '@/widgets/layout/nav-menu'
import type { SubnavPageTab } from '@/widgets/layout/nav-config'
import { PageHeaderActionsHost } from '../components/page-header-actions'
import { SubnavPills } from '../components/subnav-pills'

interface SubnavPageLayoutProps {
  /** 二级页清单（`securityTabs` / `settingsTabs`）：同时提供标签与页头文案 */
  tabs: SubnavPageTab[]
  /** 标签行与页头的无障碍名称（i18n key） */
  ariaLabelKey: string
  /**
   * 二级导航形态：
   *   - `tabs`（默认）＝页头下方一条横向胶囊行（页数少、且二级之间是"同一页的不同切片"）；
   *   - `menu`＝左侧纵向菜单（页数多、或二级之间是彼此独立的页面：与组织管理的资源侧栏同形）。
   * 两种形态都由**布局**常驻渲染，因此路由切换时导航不卸载、滑块位移才有起点。
   */
  variant?: 'tabs' | 'menu'
}

/**
 * 二级页的组级布局：页头（标题 / 描述）与胶囊标签由**布局**渲染，页面只提供内容。
 *
 * 为什么要这一层：标签要像顶栏导航那样"滑"过去，而 CSS transition 只能过渡**同一个元素**——
 * 标签若由各页自己渲染，点一下换路由就整块重挂载，滑块每次都是新元素，只能瞬时落位
 * （详见 `docs/design-language.md` §5.0）。放进布局后标签在路由切换中不卸载，位移过渡才成立。
 *
 * 页面侧不变的部分：内容、以及自己的页头操作按钮（后者用 `usePageHeaderActions` portal 进来）。
 */
export function SubnavPageLayout({ tabs, ariaLabelKey, variant = 'tabs' }: SubnavPageLayoutProps) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const [actionsHost, setActionsHost] = useState<HTMLDivElement | null>(null)
  // 找不到就退回第一项（`/security` → `/security/certs` 的那一帧还没匹配上）
  const current = tabs.find((tab) => tab.isActive(pathname)) ?? tabs[0]
  const menu = variant === 'menu'

  return (
    <PageShell
      titleKey={current.titleKey}
      subtitleKey={current.subtitleKey}
      actions={
        // 页面 portal 进来的按钮挂在这里；与 PageShell 的 actions 行同一套 flex 间隙
        <div ref={setActionsHost} className="flex flex-wrap items-center gap-2" />
      }
      subnav={menu ? undefined : <SubnavPills ariaLabelKey={ariaLabelKey} tabs={tabs} />}
      sidebar={
        menu ? (
          // 纵向菜单：面板与条目都复用工作区侧栏那套（`NavMenu` = 同一块滑动墨色胶囊）
          <div className={cn(NAV_PANEL_CLASS, 'gap-1 p-2')}>
            <p className="text-muted-foreground px-3.5 pt-1 pb-0.5 text-[10px] font-semibold tracking-wide">
              {t(ariaLabelKey)}
            </p>
            <NavMenu
              ariaLabel={t(ariaLabelKey)}
              sections={[
                {
                  // 没有抬头：面板标题已经写着"系统设置 / 安全管理"
                  items: tabs.map((tab) => ({
                    to: tab.to,
                    labelKey: tab.labelKey,
                    icon: tab.icon,
                    active: tab.isActive(pathname),
                  })),
                },
              ]}
            />
          </div>
        ) : undefined
      }
    >
      <PageHeaderActionsHost.Provider value={actionsHost}>
        <Outlet />
      </PageHeaderActionsHost.Provider>
    </PageShell>
  )
}
