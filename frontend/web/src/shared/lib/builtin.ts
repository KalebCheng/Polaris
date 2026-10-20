/**
 * 内置（种子）资源标识：与后端 `internal/repository/builtin.go` 保持一致。
 * 内置组织与系统管理员账号属于系统自身，默认不可删除（后端同样会拒绝）。
 */
export const BUILT_IN_INSTITUTION = 'built-in'
export const BUILT_IN_ADMIN = 'admin'

/** 是否为内置组织 */
export function isBuiltInInstitution(name?: string): boolean {
  return name === BUILT_IN_INSTITUTION
}

/** 是否为内置系统管理员账号（内置组织下的 admin） */
export function isBuiltInAdmin(institution?: string, name?: string): boolean {
  return institution === BUILT_IN_INSTITUTION && name === BUILT_IN_ADMIN
}
