import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { AppWindow, Clock3, Fingerprint, KeyRound, Monitor, UserRound, type LucideIcon } from 'lucide-react'
import { BrandLockup } from '@/shared/components/brand-lockup'
import { AccountMenu } from '@/widgets/layout/account-menu'
import { NavPills } from '@/widgets/layout/nav-pills'
import { useCurrentInstitution } from '@/features/iam/institution-context'

interface PortalLink {
  to: string
  label: string
  icon: LucideIcon
  end?: boolean
}

const portalLinks: PortalLink[] = [
  { to: '/portal', label: 'portal.nav.apps', icon: AppWindow, end: true },
  { to: '/portal/profile', label: 'portal.nav.profile', icon: UserRound },
  { to: '/portal/security', label: 'portal.nav.security', icon: KeyRound },
  { to: '/portal/identities', label: 'portal.nav.identities', icon: Fingerprint },
  { to: '/portal/app-accounts', label: 'portal.nav.appAccounts', icon: AppWindow },
  { to: '/portal/sessions', label: 'portal.nav.sessions', icon: Monitor },
  { to: '/portal/activity', label: 'portal.nav.activity', icon: Clock3 },
]

/**
 * 用户门户外壳：与桌面控制台同一套 chrome——
 * 顶栏（品牌 + 门户标识 + 账号菜单）+ 水平一级导航 + 铺满的内容区。
 */
export function PortalLayout() {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const { logo } = useCurrentInstitution()
  const navItems = portalLinks.map(({ to, label, end }) => ({
    to,
    label: t(label),
    active: end ? pathname === to : pathname.startsWith(to),
  }))

  return (
    <div className="app-canvas flex h-full flex-col">
      {/* 与桌面控制台同一套外壳：悬浮玻璃岛 */}
      {/* 左右留白与内容区同一套刻度，玻璃岛与内容卡片左右对齐 */}
      <header className="sticky top-0 z-40 shrink-0 px-4 pt-3 sm:px-6 lg:px-8">
        <div className="bg-card/60 border-border/60 supports-[backdrop-filter]:bg-card/55 shadow-soft overflow-hidden rounded-2xl border backdrop-blur-2xl backdrop-saturate-150">
          <div className="flex h-14 items-center gap-2 px-2.5 sm:h-16 sm:gap-3 sm:px-4">
            <NavLink to="/portal" className="flex shrink-0 items-center">
              <BrandLockup src={logo} />
            </NavLink>
            <span className="bg-border mx-1 hidden h-5 w-px sm:block" aria-hidden="true" />
            <span className="text-muted-foreground hidden text-[13px] font-medium sm:block">
              {t('portal.brandShort')}
            </span>

            {/* 门户导航项较多：允许横向滚动，避免中等宽度下挤压品牌与账号区 */}
            <NavPills
              items={navItems}
              ariaLabel={t('portal.navigation')}
              className="ml-1 hidden min-w-0 overflow-x-auto md:flex"
            />

            <span className="flex-1" />
            <AccountMenu />
          </div>

          {/* 窄屏：门户导航折叠为玻璃岛内的第二行，可横向滚动 */}
          <NavPills
            items={navItems}
            ariaLabel={t('portal.navigation')}
            className="border-border/50 flex h-12 items-center gap-1 overflow-x-auto border-t px-2 md:hidden"
            itemClassName="h-8 px-3"
          />
        </div>
      </header>

      <main className="min-h-0 min-w-0 flex-1 overflow-y-auto">
        <div className="min-h-full w-full px-4 py-5 sm:px-6 sm:py-6 lg:px-8">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
