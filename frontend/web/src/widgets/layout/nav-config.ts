import {
  FileClock,
  Layers3,
  LayoutDashboard,
  Network,
  PlugZap,
  Shuffle,
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
 * 平台级导航：跨组织总览、组织管理，以及提升为平台级的
 * 角色 / 权限 / 安全管理 / 审计日志。
 */
export const platformNav: PlatformNavItem[] = [
  { to: '/', labelKey: 'nav.dashboard', isActive: (pathname) => pathname === '/' },
  { to: '/institutions', labelKey: 'nav.institutions', isActive: (pathname) => pathname.startsWith('/institutions') },
  { to: '/roles', labelKey: 'nav.roles', isActive: (pathname) => pathname === '/roles' },
  { to: '/permissions', labelKey: 'nav.permissions', isActive: (pathname) => pathname === '/permissions' },
  { to: '/security', labelKey: 'nav.security', isActive: (pathname) => pathname.startsWith('/security') },
  { to: '/audit', labelKey: 'nav.audit', isActive: (pathname) => pathname === '/audit' },
  { to: '/settings/storage', labelKey: 'nav.settings', isActive: (pathname) => pathname.startsWith('/settings') },
  // 任务中心（asyncd）：异步任务与定时调度的可见性与人工干预
  { to: '/asyncd', labelKey: 'nav.asyncd', isActive: (pathname) => pathname === '/asyncd' },
]

/** 安全管理下的子页（二级标签） */
export interface SecurityTab {
  to: string
  labelKey: string
}

export const securityTabs: SecurityTab[] = [
  { to: '/security/certs', labelKey: 'nav.certs' },
  { to: '/security/tokens', labelKey: 'nav.tokens' },
  { to: '/security/sessions', labelKey: 'nav.sessions' },
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
