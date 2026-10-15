import { readFileSync } from 'node:fs'
import { globSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import en from './locales/en.json'
import zh from './locales/zh.json'

type Tree = { [key: string]: string | Tree }

function has(tree: Tree, path: string): boolean {
  return path.split('.').reduce<string | Tree | undefined>((node, part) => {
    if (!node || typeof node === 'string') return undefined
    return node[part]
  }, tree) !== undefined
}


/** 取词条叶子值（非字符串返回 undefined） */
function leaf(tree: Tree, path: string): string | undefined {
  const value = path.split('.').reduce<string | Tree | undefined>((node, part) => {
    if (!node || typeof node === 'string') return undefined
    return node[part]
  }, tree)
  return typeof value === 'string' ? value : undefined
}

/**
 * 从源码中提取静态的 i18n key。除 t('a.b') / labelKey="a.b" 这类直接引用外，
 * 还要覆盖"key 先存进数组再传给 t(labelKey)"的写法（如开关列表），
 * 判定方式：形如 a.b 的字符串字面量，且首段是已存在的顶级命名空间。
 */
function extractKeys(source: string, namespaces: Set<string>): string[] {
  const keys: string[] = []
  const patterns = [
    /\bt\(\s*'([a-zA-Z0-9_.]+)'/g,
    /\bt\(\s*"([a-zA-Z0-9_.]+)"/g,
    /(?:labelKey|titleKey|subtitleKey|headerKey)="([a-zA-Z0-9_.]+)"/g,
    /['"]([a-z][a-zA-Z0-9]*(?:\.[a-zA-Z0-9]+)+)['"]/g,
  ]
  for (const pattern of patterns) {
    for (const match of source.matchAll(pattern)) {
      const key = match[1]
      if (!namespaces.has(key.split('.')[0])) continue
      keys.push(key)
    }
  }
  return keys
}

/**
 * 回归：代码里引用的静态 i18n key 必须存在。
 * 曾两次出现"字段标签 key 未定义"导致界面直接显示 `applications.disableSignin` 这类原始 key。
 */
describe('i18n 词条引用完整性', () => {
  const files = globSync('src/**/*.{ts,tsx}', { cwd: process.cwd() }).filter(
    (file) => !file.includes('.test.') && !file.includes('locales/'),
  )

  it('所有静态引用的 key 都能在中英词条中找到', () => {
    const namespaces = new Set(Object.keys(zh as Tree))
    const missing = new Map<string, string>()
    for (const file of files) {
      const source = readFileSync(join(process.cwd(), file), 'utf8')
      for (const key of extractKeys(source, namespaces)) {
        if (!has(zh as Tree, key) || !has(en as Tree, key)) {
          missing.set(key, file)
        }
      }
    }
    expect(Object.fromEntries(missing)).toEqual({})
  })

  /**
   * 回归：字段标签/提示按 `<key>` 与 `<key>Hint` **平铺**为字符串。
   * 曾把 connections.field.name 写成 { label, hint } 对象，界面直接渲染出
   * "key 'connections.field.name (zh)' returned an object instead of string"。
   */
  it('connections.field.* 一律是字符串（标签平铺，提示用 <key>Hint）', () => {
    const flatten = (tree: Tree, prefix = ''): string[] =>
      Object.entries(tree).flatMap(([key, value]) => {
        const path = prefix ? `${prefix}.${key}` : key
        return typeof value === 'string' ? [] : [...flatten(value, path), path]
      })
    const fieldTree = (zh as Tree)['connections']
    const nested = flatten(
      ((fieldTree as Tree)['field'] as Tree) ?? {},
      'connections.field',
    )
    expect(nested).toEqual([])
    // 类型与变体的文案以 .name / .desc 子键存在，这里顺带锁定其叶子必须是字符串
    const types = (zh as Tree)['connections'] as Tree
    expect(typeof ((types['types'] as Tree)['dingtalk'] as Tree)['name']).toBe('string')
    expect(typeof ((types['variants'] as Tree)['ad'] as Tree)['desc']).toBe('string')
  })

  /**
   * 回归：被当作**字符串**使用的 key 必须解析为字符串。
   * 曾出现"字段标签写成嵌套对象（如 connections.field.name = { label, hint }），
   * 代码却按字符串取用"，界面上直接显示 `key 'connections.field.name (zh)' returned an object instead of string`。
   */
  it('静态引用的 key 解析结果必须是字符串（不得是嵌套对象）', () => {
    const namespaces = new Set(Object.keys(zh as Tree))
    const wrong = new Map<string, string>()
    for (const file of files) {
      const source = readFileSync(join(process.cwd(), file), 'utf8')
      for (const key of extractKeys(source, namespaces)) {
        if (!has(zh as Tree, key)) continue
        if (leaf(zh as Tree, key) === undefined || leaf(en as Tree, key) === undefined) {
          // 排除"仅用于拼接子键"的命名空间前缀（如 connections.types.dingtalk）
          continue
        }
      }
      // 反向检查：源码中明确以字符串形式取用的 key（t('...') / labelKey="..."）不得是对象
      const stringUsages = [
        ...source.matchAll(/\bt\(\s*['"]([a-zA-Z0-9_.]+)['"]/g),
        ...source.matchAll(/(?:labelKey|titleKey|subtitleKey|headerKey)="([a-zA-Z0-9_.]+)"/g),
      ]
      for (const match of stringUsages) {
        const key = match[1]
        if (!namespaces.has(key.split('.')[0]) || !has(zh as Tree, key)) continue
        if (leaf(zh as Tree, key) === undefined || leaf(en as Tree, key) === undefined) {
          wrong.set(key, file)
        }
      }
    }
    expect(Object.fromEntries(wrong)).toEqual({})
  })
})
