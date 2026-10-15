import { useTranslation } from 'react-i18next'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { cn } from '@/shared/lib/utils'

interface ListPaginationProps {
  page: number
  size: number
  total: number
  onPageChange: (page: number) => void
  /** 左侧范围文案的资源量词，如「条」「个组织」 */
  unit?: string
}

/** 生成页码窗口：首尾 + 当前附近的省略号 */
function pageWindow(page: number, pages: number): (number | 'ellipsis')[] {
  if (pages <= 7) return Array.from({ length: pages }, (_, index) => index + 1)
  const result: (number | 'ellipsis')[] = [1]
  const start = Math.max(2, page - 1)
  const end = Math.min(pages - 1, page + 1)
  if (start > 2) result.push('ellipsis')
  for (let index = start; index <= end; index += 1) result.push(index)
  if (end < pages - 1) result.push('ellipsis')
  result.push(pages)
  return result
}

/** 列表分页：左侧显示范围与总数，右侧为页码 */
export function ListPagination({ page, size, total, onPageChange, unit }: ListPaginationProps) {
  const { t } = useTranslation()
  const pages = Math.max(1, Math.ceil(total / size))
  const current = Math.min(Math.max(1, page), pages)
  const from = total === 0 ? 0 : (current - 1) * size + 1
  const to = Math.min(current * size, total)
  const suffix = unit ? unit : t('common.itemsUnit')

  const buttonClass =
    'grid size-8 place-items-center rounded-full text-[12px] tabular-nums transition-colors disabled:opacity-40'

  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <p className="text-muted-foreground text-[11px]">
        {t('common.rangeSummary', { from, to, total, unit: suffix })}
      </p>
      {pages > 1 ? (
        <div className="flex items-center gap-1.5">
          <button
            type="button"
            className={cn(buttonClass, 'text-muted-foreground hover:bg-accent hover:text-foreground')}
            disabled={current <= 1}
            onClick={() => onPageChange(current - 1)}
            aria-label={t('common.previous')}
          >
            <ChevronLeft className="size-3.5" />
          </button>
          {pageWindow(current, pages).map((item, index) =>
            item === 'ellipsis' ? (
              <span key={`gap-${index}`} className="text-muted-foreground px-1 text-[12px]">
                …
              </span>
            ) : (
              <button
                key={item}
                type="button"
                aria-current={item === current ? 'page' : undefined}
                className={cn(
                  buttonClass,
                  item === current
                    ? 'bg-ink text-ink-foreground font-medium'
                    : 'text-muted-foreground hover:bg-accent hover:text-foreground',
                )}
                onClick={() => onPageChange(item)}
              >
                {item}
              </button>
            ),
          )}
          <button
            type="button"
            className={cn(buttonClass, 'text-muted-foreground hover:bg-accent hover:text-foreground')}
            disabled={current >= pages}
            onClick={() => onPageChange(current + 1)}
            aria-label={t('common.next')}
          >
            <ChevronRight className="size-3.5" />
          </button>
        </div>
      ) : null}
    </div>
  )
}
