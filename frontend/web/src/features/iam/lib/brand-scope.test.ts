import { describe, expect, it } from 'vitest'
import { resolveBrandScope } from './brand-scope'

/**
 * 回归：管理端控制台一律使用平台品牌——包括点进组织后的工作区页面。
 * 曾导致系统管理员在管理端看到「上次访问组织」的 logo 与 favicon。
 */
describe('品牌作用域解析', () => {
  it('管理端控制台（含组织工作区）使用平台品牌', () => {
    for (const path of [
      '/',
      '/institutions',
      '/institutions/new',
      '/institutions/acme/edit',
      '/institutions/acme',
      '/institutions/acme/users',
      '/institutions/acme/applications/portal?tab=1',
      '/roles',
      '/permissions',
      '/security/certs',
      '/audit',
      '/settings/storage',
      '/login',
      '/signup',
      '/magic',
      '/sso/callback',
    ]) {
      expect(resolveBrandScope(path)).toBe('platform')
    }
  })

  it('用户门户使用所属组织品牌', () => {
    for (const path of ['/portal', '/portal/', '/portal/profile', '/portal/application-accounts?page=2']) {
      expect(resolveBrandScope(path)).toBe('tenant')
    }
  })
})
