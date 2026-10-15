import { describe, expect, it } from 'vitest'
import type { ConnectionTypeInfo } from '@/shared/api/types'
import { toCatalogItems } from './connection-catalog'

function type(over: Partial<ConnectionTypeInfo>): ConnectionTypeInfo {
  return {
    key: 'dingtalk',
    direction: 'in',
    name: '钉钉',
    description: 'desc',
    icon: { key: 'dingtalk' },
    fields: [],
    capabilities: {
      mapping: false,
      incremental: false,
      group_scope: true,
      soft_delete: true,
      test: true,
      target_department: true,
    },
    ...over,
  }
}

describe('toCatalogItems', () => {
  it('无变体的类型展开成一张卡片，名称与说明走 i18n 词条', () => {
    const items = toCatalogItems([type({})])
    expect(items).toHaveLength(1)
    expect(items[0].name).toBe('connections.types.dingtalk.name')
    expect(items[0].description).toBe('connections.types.dingtalk.desc')
  })

  it('带变体的类型按变体展开（LDAP/AD、数据库、飞书/Lark）', () => {
    const items = toCatalogItems([
      type({
        key: 'ldap',
        icon: { key: 'ldap' },
        variants: [
          { key: 'ad', name: 'AD', description: 'd', icon: 'ad' },
          { key: 'ldap', name: 'OpenLDAP', description: 'd', icon: 'ldap' },
        ],
      }),
      type({
        key: 'database',
        icon: { key: 'database', by_sub_type: { mysql: 'mysql', postgres: 'postgres' } },
        variants: [
          { key: 'mysql', name: 'MySQL', description: 'd', icon: 'mysql' },
          { key: 'postgres', name: 'PostgreSQL', description: 'd', icon: 'postgres' },
        ],
      }),
    ])
    expect(items.map((item) => `${item.typeKey}:${item.subType}`)).toEqual([
      'ldap:ad',
      'ldap:ldap',
      'database:mysql',
      'database:postgres',
    ])
    expect(items[0].iconKey).toBe('ad')
    expect(items[3].name).toBe('connections.variants.postgres.name')
  })

  it('未登记词条的类型回退后端文案（不出现空卡片）', () => {
    const items = toCatalogItems([
      type({ key: 'future-type', name: '未来类型', description: '后端文案' }),
    ])
    expect(items[0].name).toBe('conn:未来类型')
    expect(items[0].description).toBe('conn:后端文案')
  })
})
