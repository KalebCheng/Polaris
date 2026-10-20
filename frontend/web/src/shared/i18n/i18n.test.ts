import { describe, expect, it } from 'vitest'
import en from './locales/en.json'
import zh from './locales/zh.json'

type Tree = { [key: string]: string | Tree }

/** 展平嵌套的 i18n 资源为 "a.b.c" 路径集合 */
function flatten(tree: Tree, prefix = ''): string[] {
  return Object.entries(tree).flatMap(([key, value]) => {
    const path = prefix ? `${prefix}.${key}` : key
    return typeof value === 'string' ? [path] : flatten(value, path)
  })
}

function emptyKeys(tree: Tree, prefix = ''): string[] {
  return Object.entries(tree).flatMap(([key, value]) => {
    const path = prefix ? `${prefix}.${key}` : key
    if (typeof value === 'string') return value.trim() === '' ? [path] : []
    return emptyKeys(value, path)
  })
}

/**
 * 回归：双语必须严格对齐。
 * 曾出现只补中文 key、英文缺失导致界面回退显示中文的问题（AGENTS.md §7.3-5）。
 */
describe('i18n 资源一致性', () => {
  const zhKeys = flatten(zh as Tree).toSorted()
  const enKeys = flatten(en as Tree).toSorted()

  it('中英 key 集合完全一致', () => {
    const missingInEn = zhKeys.filter((key) => !enKeys.includes(key))
    const missingInZh = enKeys.filter((key) => !zhKeys.includes(key))
    expect({ missingInEn, missingInZh }).toEqual({ missingInEn: [], missingInZh: [] })
  })

  it('不存在空文案', () => {
    expect(emptyKeys(zh as Tree)).toEqual([])
    expect(emptyKeys(en as Tree)).toEqual([])
  })

  it('导航分组与关键页面 key 齐备', () => {
    for (const key of [
      'nav.overview',
      'nav.directory',
      'nav.apps',
      'nav.security',
      'nav.audit',
      'nav.profile',
      'nav.groupOverview',
      'users.status',
    ]) {
      expect(zhKeys).toContain(key)
    }
  })
})
