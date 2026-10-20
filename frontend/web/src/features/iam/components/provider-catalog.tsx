import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import { Button } from '@/shared/ui'
import { ProviderTile } from '@/shared/components/provider-icon'
import type { ProviderCategory, ProviderTypeInfo } from '@/shared/api/types'

/**
 * 提供商目录：按类别分节，每个类型一张卡片（图标 + 名称 + 一句话说明 + 「添加」）。
 * 类别与类型全部来自后端目录（GET /providers/catalog），前端不做任何硬编码。
 */

export interface ProviderCatalogItem {
  categoryKey: string
  typeKey: string
  iconKey: string
  name: string
  description: string
}

/** 把后端目录展开为卡片清单（类别顺序即目录顺序） */
export function toCatalogItems(categories: ProviderCategory[]): ProviderCatalogItem[] {
  const items: ProviderCatalogItem[] = []
  for (const category of categories) {
    for (const type of category.types) {
      items.push({
        categoryKey: category.key,
        typeKey: type.key,
        iconKey: type.icon,
        name: type.name,
        description: type.description,
      })
    }
  }
  return items
}

interface ProviderCatalogGridProps {
  items: ProviderCatalogItem[]
  onPick: (item: ProviderCatalogItem) => void
}

/** 目录网格：响应式 3 列卡片 */
export function ProviderCatalogGrid({ items, onPick }: ProviderCatalogGridProps) {
  const { t } = useTranslation()
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {items.map((item) => (
        <div
          key={`${item.categoryKey}:${item.typeKey}`}
          className="surface hover:border-primary/40 flex flex-col gap-3 rounded-2xl p-4 transition-[border-color,box-shadow]"
        >
          <div className="flex items-center gap-3">
            <ProviderTile iconKey={item.iconKey} tileSize={40} />
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold">{item.name}</p>
              <p className="text-muted-foreground truncate text-[11px]">
                {t(`providers.category.${item.categoryKey}`)}
              </p>
            </div>
          </div>
          <p className="text-muted-foreground min-h-[34px] text-xs leading-relaxed">{item.description}</p>
          <Button type="button" size="sm" className="self-start" onClick={() => onPick(item)}>
            <Plus className="size-4" /> {t('providers.addThisType')}
          </Button>
        </div>
      ))}
    </div>
  )
}

/** 按类别分组的目录（用于「新增」页：一次看全类别，不必先切标签） */
export function groupByCategory(
  categories: ProviderCategory[],
  items: ProviderCatalogItem[],
): { category: ProviderCategory; items: ProviderCatalogItem[] }[] {
  return categories
    .map((category) => ({
      category,
      items: items.filter((item) => item.categoryKey === category.key),
    }))
    .filter((group) => group.items.length > 0)
}

export type { ProviderTypeInfo }
