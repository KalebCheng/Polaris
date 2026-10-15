import { describe, expect, it } from 'vitest'
import { platformNav, settingsTabs } from './nav-config'

/**
 * 回归：任务中心与审计日志**是「设置」下的二级页**，不是一级导航。
 * 两者各占一个一级导航位时，顶栏被运维入口占满，平台级资源（组织 / 角色 / 权限 / 安全）反而被挤到后面；
 * 若有人把它们加回 `platformNav`，一级地址就会与二级页并存，快速跳转里会出现两条同址结果。
 */
describe('平台级导航结构', () => {
  it('一级导航不含 /audit 与 /asyncd，「设置」指向首个二级页', () => {
    const targets = platformNav.map((item) => item.to)
    expect(targets).not.toContain('/audit')
    expect(targets).not.toContain('/asyncd')
    expect(targets).toContain(settingsTabs[0].to)
  })

  it('设置的二级页＝存储设置 / 任务中心 / 审计日志，且各自路径上高亮', () => {
    expect(settingsTabs.map((tab) => tab.to)).toEqual(['/settings/storage', '/settings/tasks', '/settings/audit'])
    for (const tab of settingsTabs) {
      expect(tab.isActive(tab.to)).toBe(true)
    }
    // /settings 会重定向到存储设置，此时也应当高亮「存储设置」
    expect(settingsTabs[0].isActive('/settings')).toBe(true)
  })

  it('任一设置路径只让一个胶囊高亮（不会出现两个选中项）', () => {
    for (const path of ['/settings', '/settings/storage', '/settings/tasks', '/settings/audit']) {
      expect(settingsTabs.filter((tab) => tab.isActive(path))).toHaveLength(1)
    }
  })
})
