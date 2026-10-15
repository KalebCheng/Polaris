/**
 * 品牌作用域：决定同一套界面该用谁的品牌。
 *
 * - `tenant`：**面向终端用户**的入口（用户门户 `/portal/**`）——属于具体组织，用该组织的 Logo/Favicon。
 *   （登录页 / 注册页 / 魔法链接落地页不走这里：它们由 `/auth/app-info` 直接按应用与组织渲染品牌。）
 * - `platform`：**其余全部页面**（含整个管理端控制台：平台层的 Dashboard / 组织 / 角色 / 权限 / 安全 /
 *   审计 / 设置，**以及**点进组织后的工作区 `/institutions/:inst/**`）——控制台是平台自身的产品界面，
 *   一律使用平台内置品牌（Polaris）。
 *
 * 历史教训：曾用「上次访问过的组织」（localStorage）驱动控制台顶栏品牌，
 * 导致系统管理员在管理端看到某个租户的图片、浏览器标签也变成该组织的 favicon。
 */
export type BrandScope = 'platform' | 'tenant'

/** 依据当前路径解析品牌作用域（纯函数，便于单测） */
export function resolveBrandScope(pathname: string): BrandScope {
  const clean = pathname.split('?')[0]
  if (clean === '/portal' || clean.startsWith('/portal/')) return 'tenant'
  return 'platform'
}
