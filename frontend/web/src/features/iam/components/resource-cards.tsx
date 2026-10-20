import { Fragment } from 'react'
import { useTranslation } from 'react-i18next'
import { EmptyState, Skeleton } from '@/shared/ui'
import { ListPagination } from '@/widgets/layout/list-pagination'

/**
 * 资源卡片外壳：画布上的玻璃浮层 + 悬停抬起，与提供商目录 / 身份连接卡片同一套材质。
 * 固定 `flex h-full flex-col`——同一行的卡片等高，页脚用 `mt-auto` 贴底对齐。
 */
export const RESOURCE_CARD_CLASS =
  'surface hover:border-primary/40 hover:shadow-lifted flex h-full flex-col gap-4 rounded-2xl p-4 transition-[border-color,box-shadow]'

interface ResourceCardsProps<T> {
  items: T[]
  /** 单卡渲染：卡片自身负责 `flex flex-col` 与页脚 `mt-auto`，保证同一行的卡片等高对齐 */
  renderCard: (item: T) => React.ReactNode
  cardKey: (item: T) => string | number
  loading: boolean
  page: number
  size: number
  total: number
  onPageChange: (page: number) => void
  toolbar?: React.ReactNode
  /** 空态文案：默认是通用的「暂无数据」，业务页有自己的引导语时传进来 */
  emptyTitle?: string
  emptyDescription?: string
  emptyAction?: React.ReactNode
  /** 分页范围文案量词，如「个组织」 */
  unit?: string
}

/**
 * 资源列表通用卡片网格：加载骨架 / 空状态 / 分页与 `ResourceTable` 一致，页面只提供单卡内容。
 *
 * 与表格的分工：**表格用于逐列比对，卡片用于逐个识别**。组织与应用都带 Logo、名称、标识与
 * 状态徽标，摊成一行时横向空间被列均分，反而看不清"这是个什么"；卡片把同一资源的信息收拢成
 * 一块，一眼认得出对象，也放得下状态与操作。列多、需要逐项比对的资源（用户、审计、令牌）仍用表格。
 */
export function ResourceCards<T>({
  items,
  renderCard,
  cardKey,
  loading,
  page,
  size,
  total,
  onPageChange,
  toolbar,
  emptyTitle,
  emptyDescription,
  emptyAction,
  unit,
}: ResourceCardsProps<T>) {
  const { t } = useTranslation()
  const gridClass = 'grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4'

  return (
    <div className="flex flex-col gap-4">
      {toolbar}
      {loading ? (
        <div className={gridClass}>
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className="h-48 rounded-2xl" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="surface rounded-2xl">
          <EmptyState
            title={emptyTitle ?? t('common.empty')}
            description={emptyDescription ?? t('common.emptyHint')}
            action={emptyAction}
          />
        </div>
      ) : (
        <div className={gridClass}>
          {items.map((item) => (
            // Fragment 占 key，卡片本身仍是网格子项（能吃到等高拉伸）
            <Fragment key={cardKey(item)}>{renderCard(item)}</Fragment>
          ))}
        </div>
      )}
      {!loading && items.length > 0 ? (
        <ListPagination page={page} size={size} total={total} onPageChange={onPageChange} unit={unit} />
      ) : null}
    </div>
  )
}
