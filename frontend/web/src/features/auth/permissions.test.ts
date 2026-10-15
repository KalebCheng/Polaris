import { describe, expect, it } from 'vitest'

/**
 * 前端权限判定逻辑（与后端 Enforce 保持一致的语义）：
 * 管理员直通；Deny 优先于 Allow；支持 * 通配。
 */
function can(
  status: { authenticated: boolean; is_admin?: boolean; permissions?: { resource: string; action: string; effect: string }[] },
  resource: string,
  action: string,
): boolean {
  if (!status.authenticated) return false
  if (status.is_admin) return true
  let allowed = false
  for (const rule of status.permissions ?? []) {
    if (rule.resource !== '*' && rule.resource !== resource) continue
    if (rule.action !== '*' && rule.action !== action) continue
    if (rule.effect === 'Deny') return false
    allowed = true
  }
  return allowed
}

describe('权限判定', () => {
  it('未认证一律拒绝', () => {
    expect(can({ authenticated: false }, 'user', 'read')).toBe(false)
  })

  it('管理员直通', () => {
    expect(can({ authenticated: true, is_admin: true }, 'anything', 'delete')).toBe(true)
  })

  it('通配资源允许', () => {
    const status = {
      authenticated: true,
      permissions: [{ resource: '*', action: 'read', effect: 'Allow' }],
    }
    expect(can(status, 'user', 'read')).toBe(true)
    expect(can(status, 'user', 'write')).toBe(false)
  })

  it('精确 Deny 覆盖通配 Allow', () => {
    const status = {
      authenticated: true,
      permissions: [
        { resource: '*', action: '*', effect: 'Allow' },
        { resource: 'role', action: 'write', effect: 'Deny' },
      ],
    }
    expect(can(status, 'user', 'write')).toBe(true)
    expect(can(status, 'role', 'write')).toBe(false)
  })
})
