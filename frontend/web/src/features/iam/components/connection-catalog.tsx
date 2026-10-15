import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import { Button } from '@/shared/ui'
import { ConnectionTile } from '@/shared/components/connection-icon'
import type { ConnectionTypeInfo } from '@/shared/api/types'

/**
 * 目录卡片（对齐参考产品「添加身份提供方」）：
 * 每个接入方式一张卡片 = 图标 + 名称 + 一句话说明 + 「添加」。
 * 带变体的类型（LDAP/AD、数据库、飞书/Lark）按变体展开成多张卡片。
 */

/** 类型名 i18n key（静态登记，避免动态拼接导致词条缺失无人发现） */
const TYPE_NAME_KEYS: Record<string, string> = {
  dingtalk: 'connections.types.dingtalk.name',
  wecom: 'connections.types.wecom.name',
  lark: 'connections.types.lark.name',
  ldap: 'connections.types.ldap.name',
  database: 'connections.types.database.name',
  http: 'connections.types.http.name',
}

/** 类型说明 i18n key */
const TYPE_DESC_KEYS: Record<string, string> = {
  dingtalk: 'connections.types.dingtalk.desc',
  wecom: 'connections.types.wecom.desc',
  lark: 'connections.types.lark.desc',
  ldap: 'connections.types.ldap.desc',
  database: 'connections.types.database.desc',
  http: 'connections.types.http.desc',
}

/** 变体名 i18n key */
const VARIANT_NAME_KEYS: Record<string, string> = {
  ad: 'connections.variants.ad.name',
  ldap: 'connections.variants.ldap.name',
  feishu: 'connections.variants.feishu.name',
  lark: 'connections.variants.lark.name',
  mysql: 'connections.variants.mysql.name',
  postgres: 'connections.variants.postgres.name',
}

/** 变体说明 i18n key */
const VARIANT_DESC_KEYS: Record<string, string> = {
  ad: 'connections.variants.ad.desc',
  ldap: 'connections.variants.ldap.desc',
  feishu: 'connections.variants.feishu.desc',
  lark: 'connections.variants.lark.desc',
  mysql: 'connections.variants.mysql.desc',
  postgres: 'connections.variants.postgres.desc',
}

/** 一个可添加的接入方式（类型 + 可选变体） */
export interface ConnectionCatalogItem {
  typeKey: string
  subType?: string
  iconKey: string
  name: string
  description: string
}

/** 把类型目录展开为卡片清单（变体优先，无变体则类型自身一张卡片） */
export function toCatalogItems(types: ConnectionTypeInfo[]): ConnectionCatalogItem[] {
  const items: ConnectionCatalogItem[] = []
  for (const type of types) {
    const typeName = TYPE_NAME_KEYS[type.key] ? type.key : ''
    const typeDesc = TYPE_DESC_KEYS[type.key] ? type.key : ''
    if (!type.variants?.length) {
      items.push({
        typeKey: type.key,
        iconKey: type.icon.key,
        name: typeName ? `connections.types.${typeName}.name` : `conn:${type.name}`,
        description: typeDesc ? `connections.types.${typeDesc}.desc` : `conn:${type.description}`,
      })
      continue
    }
    for (const variant of type.variants) {
      items.push({
        typeKey: type.key,
        subType: variant.key,
        iconKey: variant.icon,
        name: VARIANT_NAME_KEYS[variant.key]
          ? `connections.variants.${variant.key}.name`
          : `conn:${variant.name}`,
        description: VARIANT_DESC_KEYS[variant.key]
          ? `connections.variants.${variant.key}.desc`
          : `conn:${variant.description}`,
      })
    }
  }
  return items
}

interface ConnectionCatalogGridProps {
  items: ConnectionCatalogItem[]
  onPick: (item: ConnectionCatalogItem) => void
}

/** 目录网格：响应式 3 列卡片 */
export function ConnectionCatalogGrid({ items, onPick }: ConnectionCatalogGridProps) {
  const { t } = useTranslation()
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {items.map((item) => {
        const transKey = item.name.startsWith('connections.') ? item.name : ''
        const descKey = item.description.startsWith('connections.') ? item.description : ''
        return (
          <div
            key={`${item.typeKey}:${item.subType ?? ''}`}
            className="bg-card hover:border-primary/30 shadow-soft flex flex-col gap-3 rounded-2xl border border-border/70 p-4 transition-colors"
          >
            <div className="flex items-center gap-3">
              <ConnectionTile icon={{ key: item.iconKey }} tileSize={40} />
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold">
                  {transKey ? t(transKey) : item.name.replace(/^conn:/, '')}
                </p>
                <p className="text-muted-foreground truncate text-[11px]">
                  {item.typeKey}
                  {item.subType ? ` · ${item.subType}` : ''}
                </p>
              </div>
            </div>
            <p className="text-muted-foreground min-h-[34px] text-xs leading-relaxed">
              {descKey ? t(descKey) : item.description.replace(/^conn:/, '')}
            </p>
            <Button type="button" size="sm" className="self-start" onClick={() => onPick(item)}>
              <Plus className="size-4" /> {t('connections.add')}
            </Button>
          </div>
        )
      })}
    </div>
  )
}
