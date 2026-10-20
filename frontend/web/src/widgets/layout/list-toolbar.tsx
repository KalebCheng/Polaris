import { useTranslation } from 'react-i18next'
import { Check, ChevronDown, ListFilter, Search } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Input,
} from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

export interface ToolbarFilter {
  /** 触发器上的前缀说明，如「状态」「部门」 */
  label: string
  value: string
  options: { value: string; label: string }[]
  onChange: (value: string) => void
  /** 选中「全部」时展示的文案 */
  allLabel?: string
}

interface ListToolbarProps {
  keyword: string
  onKeywordChange: (value: string) => void
  onSubmit?: () => void
  placeholder?: string
  filters?: ToolbarFilter[]
  /** 右侧计数文案（如「共 1,248 个用户」） */
  count?: React.ReactNode
}

/** 列表工具栏：搜索框 + 可选筛选下拉 + 右侧计数 */
export function ListToolbar({
  keyword,
  onKeywordChange,
  onSubmit,
  placeholder,
  filters = [],
  count,
}: ListToolbarProps) {
  const { t } = useTranslation()
  const hasFilters = filters.length > 0

  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex flex-1 flex-col gap-2 sm:flex-row sm:items-center">
        <form
          className="relative w-full sm:max-w-sm"
          onSubmit={(event) => {
            event.preventDefault()
            onSubmit?.()
          }}
          role="search"
        >
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            value={keyword}
            onChange={(event) => onKeywordChange(event.target.value)}
            placeholder={placeholder ?? t('common.searchPlaceholder')}
            className="bg-muted/60 focus-visible:bg-card h-10 rounded-full pl-9"
            aria-label={t('common.search')}
          />
        </form>

        {hasFilters ? (
          <div className="flex flex-wrap items-center gap-2">
            {filters.map((filter) => {
              const active = Boolean(filter.value)
              const selected = filter.options.find((option) => option.value === filter.value)
              return (
                <DropdownMenu key={filter.label}>
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      className={cn(
                        'border-border/70 hover:bg-accent inline-flex h-10 items-center gap-1.5 rounded-full border px-3.5 text-[13px] transition-colors',
                        active
                          ? 'border-primary/30 text-primary bg-primary/8'
                          : 'bg-card text-muted-foreground',
                      )}
                    >
                      <ListFilter className="size-3.5" />
                      {selected ? selected.label : filter.label}
                      <ChevronDown className="size-3.5 opacity-60" />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" className="min-w-40">
                    <DropdownMenuLabel className="text-muted-foreground text-[11px] font-medium">
                      {filter.label}
                    </DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onClick={() => filter.onChange('')}>
                      <span className="flex-1">{filter.allLabel ?? t('common.all')}</span>
                      {!active ? <Check className="size-4" /> : null}
                    </DropdownMenuItem>
                    {filter.options.map((option) => (
                      <DropdownMenuItem key={option.value} onClick={() => filter.onChange(option.value)}>
                        <span className="flex-1">{option.label}</span>
                        {filter.value === option.value ? <Check className="size-4" /> : null}
                      </DropdownMenuItem>
                    ))}
                  </DropdownMenuContent>
                </DropdownMenu>
              )
            })}
          </div>
        ) : null}
      </div>

      {count ? (
        <div className="text-muted-foreground shrink-0 text-[11px] whitespace-nowrap">{count}</div>
      ) : null}
    </div>
  )
}
