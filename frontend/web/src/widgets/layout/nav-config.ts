import {
  FileClock,
  HardDrive,
  Layers3,
  LayoutDashboard,
  Network,
  PlugZap,
  ScrollText,
  Shuffle,
  Timer,
  UserCog,
  Users,
  UsersRound,
  Waypoints,
  type LucideIcon,
} from 'lucide-react'

/** 顶部一级导航项（平台级，跨组织） */
export interface PlatformNavItem {
  to: string
  labelKey: string
  /** 当前路径是否属于该导航（用于高亮） */
  isActive: (pathname: string) => boolean
}

/**
 * 平台级导航：跨组织总览、组织管理，以及提升为平台级的角色 / 权限 / 安全管理。
 *
 * 「任务中心」与「审计日志」**不是**一级导航：它们已经收进「设置」下（见 `settingsTabs`），
 * 顶栏才有余量承载真正的平台级资源；旧地址 `/asyncd`、`/audit` 一律重定向到新的二级地址。
 */
export const platformNav: PlatformNavItem[] = [
  { to: '/', labelKey: 'nav.dashboard', isActive: (pathname) => pathname === '/' },
  { to: '/institutions', labelKey: 'nav.institutions', isActive: (pathname) => pathname.startsWith('/institutions') },
  { to: '/roles', labelKey: 'nav.roles', isActive: (pathname) => pathname === '/roles' },
  { to: '/permissions', labelKey: 'nav.permissions', isActive: (pathname) => pathname === '/permissions' },
  { to: '/security', labelKey: 'nav.security', isActive: (pathname) => pathname.startsWith('/security') },
  { to: '/settings/storage', labelKey: 'nav.settings', isActive: (pathname) => pathname.startsWith('/settings') },
]

/**
 * 二级页清单的一条：**同时**驱动两件事——
 * 1) 页头里的胶囊标签（`SubnavPills` 渲染，用 `to` / `labelKey` / `isActive`）；
 * 2) 页头本身（`titleKey` / `subtitleKey`，由组级布局 `SubnavPageLayout` 渲染）。
 *
 * 为什么页头也在这里声明：二级页的页头必须由**布局**渲染，标签才会在路由切换中不卸载
 * （否则滑块每次都是新元素，只能瞬时落位，见 docs/design-language.md §5.0）。
 */
export interface SubnavPageTab {
  to: string
  labelKey: string
  /** 页头标题（独立词条，不复用页面级 key） */
  titleKey: string
  /** 页头描述（一句话口径） */
  subtitleKey: string
  /** 当前路径是否属于该二级页（用于高亮） */
  isActive: (pathname: string) => boolean
  /**
   * 行首图标：**纵向菜单形态必需**（`variant="menu"`，见 SubnavPageLayout）。
   * 横向胶囊形态用不到图标，因此该字段可选。
   */
  icon?: LucideIcon
}

/**
 * 「设置」下的二级页：平台默认存储 / 任务中心（asyncd）/ 审计日志。
 *
 * 为什么把任务中心与审计日志放在这里：它们都是**平台运维视角**的能力（队列可观测性、操作追溯），
 * 不是组织资源，也不是独立的一等资源——放在设置下既保留了可见性，又不让顶栏被运维入口占满。
 */
export const settingsTabs: SubnavPageTab[] = [
  {
    to: '/settings/storage',
    labelKey: 'storage.title',
    icon: HardDrive,
    titleKey: 'storage.title',
    subtitleKey: 'storage.subtitle',
    isActive: (pathname) => pathname === '/settings' || pathname.startsWith('/settings/storage'),
  },
  {
    to: '/settings/tasks',
    labelKey: 'nav.asyncd',
    icon: Timer,
    titleKey: 'asyncd.title',
    subtitleKey: 'asyncd.subtitle',
    isActive: (pathname) => pathname.startsWith('/settings/tasks'),
  },
  {
    to: '/settings/audit',
    labelKey: 'nav.audit',
    icon: ScrollText,
    titleKey: 'records.title',
    subtitleKey: 'records.platformSubtitle',
    isActive: (pathname) => pathname.startsWith('/settings/audit'),
  },
]

/** 安全管理下的二级页：证书 / 令牌 / 会话 */
export const securityTabs: SubnavPageTab[] = [
  {
    to: '/security/certs',
    labelKey: 'nav.certs',
    titleKey: 'certs.title',
    subtitleKey: 'certs.platformSubtitle',
    isActive: (pathname) => pathname.startsWith('/security/certs'),
  },
  {
    to: '/security/tokens',
    labelKey: 'nav.tokens',
    titleKey: 'tokens.title',
    subtitleKey: 'tokens.platformSubtitle',
    isActive: (pathname) => pathname.startsWith('/security/tokens'),
  },
  {
    to: '/security/sessions',
    labelKey: 'nav.sessions',
    titleKey: 'sessions.title',
    subtitleKey: 'sessions.platformSubtitle',
    isActive: (pathname) => pathname.startsWith('/security/sessions'),
  },
]

/** 组织内（工作区）导航项 */
export interface NavItem {
  to: string
  labelKey: string
  icon: LucideIcon
  /** 需要的读取权限资源；为空表示不校验 */
  resource?: string
}

export interface NavSection {
  labelKey: string
  items: NavItem[]
}

/**
 * 工作区（当前组织内）的资源导航。
 * `to` 为组织下的相对路径（'' 表示组织概览）。
 * 角色 / 权限 / 证书 / 令牌 / 会话 / 审计已提升为平台级，不在此列。
 */
export const workspaceSections: NavSection[] = [
  {
    labelKey: 'nav.groupOverview',
    items: [{ to: '', labelKey: 'nav.overview', icon: LayoutDashboard }],
  },
  {
    labelKey: 'nav.directory',
    items: [
      { to: '/users', labelKey: 'nav.users', icon: Users, resource: 'user' },
      { to: '/groups', labelKey: 'nav.groups', icon: UsersRound, resource: 'group' },
      { to: '/departments', labelKey: 'nav.departments', icon: Network, resource: 'department' },
    ],
  },
  {
    labelKey: 'nav.apps',
    items: [
      { to: '/applications', labelKey: 'nav.applications', icon: Layers3, resource: 'application' },
      { to: '/providers', labelKey: 'nav.providers', icon: Waypoints, resource: 'provider' },
      { to: '/connections', labelKey: 'nav.connections', icon: PlugZap, resource: 'syncer' },
    ],
  },
  {
    labelKey: 'nav.lifecycle',
    items: [
      { to: '/account-strategies', labelKey: 'nav.accountStrategies', icon: Shuffle, resource: 'account_strategy' },
      { to: '/app-accounts', labelKey: 'nav.appAccounts', icon: UserCog, resource: 'app_account' },
      { to: '/provisioning', labelKey: 'nav.provisioning', icon: FileClock, resource: 'provision_history' },
    ],
  },
]

/** 组织内某个相对路径对应的完整地址（组织段用主键 id） */
export function workspacePath(institutionId: string, relative: string): string {
  return institutionId ? `/institutions/${institutionId}${relative}` : '/institutions'
}
