import { describe, expect, it } from 'vitest'
import { BUILT_IN_ADMIN, BUILT_IN_INSTITUTION, isBuiltInAdmin, isBuiltInInstitution } from './builtin'

/**
 * 回归：内置组织 / 内置系统管理员账号必须被识别为受保护资源（页面据此隐藏删除入口，后端同样拒绝）。
 */
describe('内置资源保护', () => {
  it('识别内置组织', () => {
    expect(isBuiltInInstitution(BUILT_IN_INSTITUTION)).toBe(true)
    expect(isBuiltInInstitution('acme')).toBe(false)
    expect(isBuiltInInstitution('')).toBe(false)
    expect(isBuiltInInstitution(undefined)).toBe(false)
  })

  it('识别内置系统管理员（仅限内置组织下的 admin）', () => {
    expect(isBuiltInAdmin(BUILT_IN_INSTITUTION, BUILT_IN_ADMIN)).toBe(true)
    // 其它组织里的 admin 不受保护
    expect(isBuiltInAdmin('acme', BUILT_IN_ADMIN)).toBe(false)
    // 内置组织里的其它账号不受保护
    expect(isBuiltInAdmin(BUILT_IN_INSTITUTION, 'bob')).toBe(false)
    expect(isBuiltInAdmin(undefined, undefined)).toBe(false)
  })
})
