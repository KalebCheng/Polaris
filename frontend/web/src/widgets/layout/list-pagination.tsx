import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button, SlidingPill, useSlidingPill } from '@/shared/ui'
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

  // 页码切换的"墨色胶囊"与顶栏导航 / 分段控件共用同一套滑块（量测必须在容器里做）
  const pagesRef = useRef<HTMLDivElement>(null)
  const { box, ready } = useSlidingPill(pagesRef, { activeSelector: '[data-active="true"]' })

  const pageButtonClass =
    'relative z-10 size-8 rounded-full text-[12px] tabular-nums text-muted-foreground hover:bg-accent hover:text-foreground'
  // 选中项只改文字色，底色交给滑块（再自己加 bg-ink 会与滑块形成重影）
  const activePageClass = 'text-ink-foreground hover:bg-transparent hover:text-ink-foreground font-medium'

  return (
    /*
      分页吸底：长列表滚到中间时页码仍可见（不用先滚到底）。
      与页头同一套做法——负 margin 抵消布局内边距让背景铺满，blur 挡住底下的行。
    */
    <div className="bg-canvas/85 sticky bottom-0 z-10 -mx-4 flex flex-col gap-3 px-4 py-3 backdrop-blur-sm sm:-mx-6 sm:flex-row sm:items-center sm:justify-between sm:px-6 lg:-mx-8 lg:px-8">
      <p className="text-muted-foreground text-[11px]">
        {t('common.rangeSummary', { from, to, total, unit: suffix })}
      </p>
      {pages > 1 ? (
        <div ref={pagesRef} className="bg-muted/70 relative flex items-center gap-1 rounded-full p-1">
          <SlidingPill box={box} ready={ready} />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={pageButtonClass}
            disabled={current <= 1}
            onClick={() => onPageChange(current - 1)}
            aria-label={t('common.previous')}
          >
            <ChevronLeft className="size-3.5" />
          </Button>
          {pageWindow(current, pages).map((item, index) =>
            item === 'ellipsis' ? (
              <span key={`gap-${index}`} className="text-muted-foreground px-1 text-[12px]">
                …
              </span>
            ) : (
              <Button
                key={item}
                type="button"
                variant="ghost"
                size="icon"
                data-active={item === current}
                aria-current={item === current ? 'page' : undefined}
                className={cn(pageButtonClass, item === current && activePageClass)}
                onClick={() => onPageChange(item)}
              >
                {item}
              </Button>
            ),
          )}
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={pageButtonClass}
            disabled={current >= pages}
            onClick={() => onPageChange(current + 1)}
            aria-label={t('common.next')}
          >
            <ChevronRight className="size-3.5" />
          </Button>
        </div>
      ) : null}
    </div>
  )
}
