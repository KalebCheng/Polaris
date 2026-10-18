/**
 * 当前工作区组织（模块级单值）。
 *
 * 为什么需要它：组织内资源（用户 / 应用 / 职务 / 部门 / 提供方 / 数据源 / 连接器 …）在
 * 后端的路由是扁平的（/users、/applications…），目标组织由 `instOf(c)` 解析——**优先取查询串
 * `?institution=`，否则回退到登录会话所在组织**。列表接口一直带着 `?institution=`，所以正常；
 * 但 reset-password / delete / update 这类操作过去漏带了它，于是"平台管理员在别的组织工作区
 * 操作"时，后端按会话组织查找资源 → 报「用户不存在」/「资源不存在」。
 *
 * 这里把「当前工作区组织」（来自 URL /institutions/:inst）暴露成一个模块级值，
 * 由 api 层统一注入到组织内请求上，避免每处写操作都要手动记得带上组织。
 * 说明：仅用于"组织内资源"；平台级资源（角色 / 权限 / 证书 / 令牌 / 会话 / 审计）跨组织，
 * 不能注入工作区组织，其目标组织需按资源自身归属显式指定。
 */
let activeInstitution = ''

export function setActiveInstitution(name: string) {
  activeInstitution = name
}

export function getActiveInstitution(): string {
  return activeInstitution
}
