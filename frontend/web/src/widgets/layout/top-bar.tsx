import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router-dom'
import { Menu } from 'lucide-react'
import { Button, Dialog, DialogContent, DialogTitle } from '@/shared/ui'
import { BrandLockup } from '@/shared/components/brand-lockup'
import { platformNav } from './nav-config'
import { AccountMenu } from './account-menu'
import { InstitutionSwitcher } from './institution-switcher'
import { NavSearch } from './nav-search'
import { NavPills } from './nav-pills'
import { useCurrentInstitution } from '@/features/iam/institution-context'
import { WorkspaceNav } from './nav-list'

/**
 * 顶栏：品牌 + 平台级导航（胶囊）+ 全局快速跳转 + 账号菜单。
 *
 * 导航内容＝`platformNav`（Dashboard / 组织 / 角色 / 权限 / 安全管理 / 设置）；
 * 任务中心与审计日志已是「设置」下的二级页（`settingsTabs`），不再占一级导航位。
 *
 * 视觉：**悬浮玻璃岛**——不贴边，四周留出画布，圆角面板 + 半透明卡片色 + 背景模糊，
 * 画布底色从背后透上来；选中的导航项是**反相墨色胶囊**（`bg-ink`），
 * 让"我在哪"一眼可见——比下划线在浅色画布上更清晰，也和分段控件同一套语义。
 * 窄屏下平台导航折叠为可横向滚动的一行，工作区导航收进抽屉。
 *
 * 注意：内容区（main）各自独立滚动，因此这里的模糊只作用于画布本身，不"糊"内容——
 * 这也是刻意的：控制台里让表格从标题栏下面穿过会削弱表头的可读性。
 *
 * 左右留白必须与内容区**同一套刻度**（`px-4 sm:px-6 lg:px-8`）：玻璃岛的边、内容卡片的边、
 * 侧边栏浮层的边在同一条竖直线上，页面才成立"同一个工作台"。改这里就要同步
 * `platform-layout.tsx` / `workspace-layout.tsx` / `app-sidebar.tsx` / `portal-layout.tsx`。
 */
export function TopBar() {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const { brandLogo: logo } = useCurrentInstitution()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const inWorkspace = /^\/institutions\/[^/]+/.test(pathname)
  const navItems = platformNav.map((item) => ({
    to: item.to,
    label: t(item.labelKey),
    active: item.isActive(pathname),
  }))

  useEffect(() => {
    setDrawerOpen(false)
  }, [pathname])

  return (
    <header className="sticky top-0 z-40 shrink-0 px-4 pt-3 sm:px-6 lg:px-8">
      <div className="bg-card/60 border-border/60 supports-[backdrop-filter]:bg-card/55 shadow-soft overflow-hidden rounded-2xl border backdrop-blur-2xl backdrop-saturate-150">
        <div className="flex h-14 items-center gap-2 px-2.5 sm:h-16 sm:gap-3 sm:px-4">
          {/* 工作区在窄屏下的导航抽屉 */}
          {inWorkspace ? (
            <Dialog open={drawerOpen} onOpenChange={setDrawerOpen}>
              <Button
                variant="ghost"
                size="icon"
                className="-ml-1 rounded-full md:hidden"
                aria-label={t('nav.workspaceNav')}
                onClick={() => setDrawerOpen(true)}
              >
                <Menu className="size-5" />
              </Button>
              <DialogContent
                showCloseButton={false}
                innerClassName="p-0 gap-0"
                className="bg-sidebar text-sidebar-foreground top-0 left-0 h-full w-[17rem] max-w-[85vw] translate-x-0 translate-y-0 rounded-none border-y-0 border-l-0 sm:max-w-[85vw]"
              >
                <DialogTitle className="sr-only">{t('nav.workspaceNav')}</DialogTitle>
                <div className="flex h-16 items-center gap-2.5 px-4">
                  <BrandLockup src={logo} size="sm" />
                </div>
                <InstitutionSwitcher />
                <WorkspaceNav />
              </DialogContent>
            </Dialog>
          ) : null}

          <Link to="/" className="flex shrink-0 items-center pr-1">
            <BrandLockup src={logo} />
          </Link>

          <NavPills items={navItems} ariaLabel={t('nav.platform')} className="ml-1 hidden md:flex" />

          <span className="flex-1" />
          <NavSearch className="hidden w-56 lg:inline-flex" />
          <AccountMenu />
        </div>

        {/* 窄屏：平台导航折叠为玻璃岛内的第二行，可横向滚动 */}
        <NavPills
          items={navItems}
          ariaLabel={t('nav.platform')}
          className="border-border/50 flex h-12 items-center gap-1 overflow-x-auto border-t px-2 md:hidden"
          itemClassName="h-8 px-3"
        />
      </div>
    </header>
  )
}
