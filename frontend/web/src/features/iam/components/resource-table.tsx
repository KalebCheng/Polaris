import { useTranslation } from 'react-i18next'
import {
  EmptyState,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/shared/ui'
import { ListPagination } from '@/widgets/layout/list-pagination'

export interface Column<T> {
  key: string
  headerKey: string
  render: (row: T) => React.ReactNode
  className?: string
  headClassName?: string
}

interface ResourceTableProps<T> {
  columns: Column<T>[]
  rows: T[]
  rowKey: (row: T) => string | number
  loading: boolean
  page: number
  size: number
  total: number
  onPageChange: (page: number) => void
  toolbar?: React.ReactNode
  emptyAction?: React.ReactNode
  /** 空态文案：默认是通用的「暂无数据」，业务页有自己的引导语时传进来（与 {@link ResourceCards} 同口径） */
  emptyTitle?: string
  emptyDescription?: string
  /** 分页范围文案量词，如「条」「个组织」 */
  unit?: string
}

/** 资源列表通用表格：加载骨架 / 空状态 / 分页一致，页面只提供列定义 */
export function ResourceTable<T>({
  columns,
  rows,
  rowKey,
  loading,
  page,
  size,
  total,
  onPageChange,
  toolbar,
  emptyAction,
  emptyTitle,
  emptyDescription,
  unit,
}: ResourceTableProps<T>) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-4">
      {toolbar}
      <div className="surface overflow-hidden rounded-2xl">
        {loading ? (
          <div className="space-y-2 p-4">
            {Array.from({ length: 5 }).map((_, index) => (
              <Skeleton key={index} className="h-11 w-full" />
            ))}
          </div>
        ) : rows.length === 0 ? (
          <EmptyState
            title={emptyTitle ?? t('common.empty')}
            description={emptyDescription ?? t('common.emptyHint')}
            action={emptyAction}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                {columns.map((column) => (
                  <TableHead key={column.key} className={column.headClassName ?? ''}>
                    {t(column.headerKey)}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={rowKey(row)}>
                  {columns.map((column) => (
                    <TableCell key={column.key} className={column.className}>
                      {column.render(row)}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>
      {/*
        分页**不能跟着 loading 卸载**：切换页码时 loading 会短暂为 true，
        一旦卸载再挂载，滑块会重新走"首帧关过渡"的路径 → 页码胶囊瞬移而看不出缓动
        （实测踩过：页码切换没有弹性效果）。只有"确实没有数据"时才不渲染。
      */}
      {rows.length > 0 ? (
        <ListPagination page={page} size={size} total={total} onPageChange={onPageChange} unit={unit} />
      ) : null}
    </div>
  )
}
