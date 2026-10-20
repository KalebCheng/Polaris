import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ChevronDown, Plus, X } from 'lucide-react'
import { Popover, PopoverContent, PopoverPortal, PopoverTrigger } from '@radix-ui/react-popover'
import { Badge } from './badge'
import { Button } from './button'
import { Input } from './input'
import { cn } from '@/shared/lib/utils'

export interface SelectOption {
  value: string
  label: string
  /** 次要说明（如资源类别、描述） */
  hint?: string
  disabled?: boolean
}

interface MultiSelectProps {
  options: SelectOption[]
  value: string[]
  onChange: (next: string[]) => void
  placeholder?: string
  /** 空值时的说明（常与"不选即不限制"配合） */
  emptyHint?: string
  disabled?: boolean
  className?: string
  /** 已选项超过该数量后折叠为 +N */
  maxVisible?: number
  /**
   * 远程搜索：提供后**不再本地过滤**，改由调用方按关键词重新拉取 options（输入防抖 300ms）。
   * 用途：候选集合很大（如组织内的用户），一次全量拉取不现实。
   */
  onSearch?: (keyword: string) => void
}

/**
 * 下拉多选：用于资源引用型字段（身份提供方、scope、令牌字段、字段编排等）。
 * 出处：这些字段的可选值来自当前组织的资源，逐个勾选框会占满整屏，下拉选择更合适。
 */
export function MultiSelect({
  options,
  value,
  onChange,
  placeholder,
  emptyHint,
  disabled,
  className,
  maxVisible = 3,
  onSearch,
}: MultiSelectProps) {
  const [open, setOpen] = useState(false)
  const [keyword, setKeyword] = useState('')

  // 远程搜索：关键词变化后防抖通知调用方（用 ref 持有回调，避免内联函数导致每次渲染都重排定时器）
  const searchRef = useRef(onSearch)
  searchRef.current = onSearch
  useEffect(() => {
    if (!searchRef.current) return
    const timer = setTimeout(() => searchRef.current?.(keyword.trim()), 300)
    return () => clearTimeout(timer)
  }, [keyword])

  const filtered = useMemo(() => {
    if (searchRef.current) return options
    if (!keyword.trim()) return options
    const lower = keyword.trim().toLowerCase()
    return options.filter(
      (option) => option.label.toLowerCase().includes(lower) || option.value.toLowerCase().includes(lower),
    )
  }, [options, keyword])

  const selected = useMemo(
    () =>
      value.map((item) => options.find((option) => option.value === item) ?? { value: item, label: item }),
    [value, options],
  )

  const toggle = (item: string) => {
    onChange(value.includes(item) ? value.filter((entry) => entry !== item) : [...value, item])
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn(
            'border-input/70 bg-card flex min-h-10 w-full items-center gap-1.5 rounded-xl border px-2 py-1.5 text-left text-[13px] transition-[color,box-shadow,border-color] outline-none',
            'focus-visible:border-ring focus-visible:ring-ring/25 focus-visible:ring-4',
            'disabled:cursor-not-allowed disabled:opacity-50',
            className,
          )}
        >
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1">
            {selected.length === 0 ? (
              <span className="text-muted-foreground px-1">{placeholder ?? emptyHint ?? '-'}</span>
            ) : (
              <>
                {selected.slice(0, maxVisible).map((item) => (
                  <Badge key={item.value} variant="secondary" className="gap-1 whitespace-nowrap">
                    {item.label}
                    <span
                      role="button"
                      tabIndex={-1}
                      className="hover:text-destructive"
                      onClick={(event) => {
                        event.stopPropagation()
                        toggle(item.value)
                      }}
                    >
                      <X className="size-3" />
                    </span>
                  </Badge>
                ))}
                {selected.length > maxVisible ? (
                  <Badge variant="outline">+{selected.length - maxVisible}</Badge>
                ) : null}
              </>
            )}
          </div>
          <ChevronDown className="size-4 shrink-0 opacity-60" />
        </button>
      </PopoverTrigger>
      {/* 必须 Portal：表单卡片是 surface overflow-hidden，内联浮层会被裁切、被后续卡片盖住 */}
      <PopoverPortal>
        <PopoverContent
          align="start"
          sideOffset={4}
          className="bg-popover text-popover-foreground border-border/70 shadow-pop z-50 w-[var(--radix-popover-trigger-width)] min-w-64 rounded-xl border p-2"
        >
          <Input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="搜索…"
            className="mb-2 h-8"
          />
          <div className="max-h-60 overflow-y-auto">
            {filtered.length === 0 ? (
              <p className="text-muted-foreground px-2 py-6 text-center text-xs">无匹配项</p>
            ) : (
              filtered.map((option) => {
                const checked = value.includes(option.value)
                return (
                  <button
                    key={option.value}
                    type="button"
                    disabled={option.disabled}
                    onClick={() => toggle(option.value)}
                    className={cn(
                      'hover:bg-accent flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] disabled:pointer-events-none disabled:opacity-50',
                    )}
                  >
                    <span
                      className={cn(
                        'flex size-4 shrink-0 items-center justify-center rounded-[4px] border',
                        checked && 'bg-primary text-primary-foreground border-primary',
                      )}
                    >
                      {checked ? <Check className="size-3" /> : null}
                    </span>
                    <span className="min-w-0 flex-1 truncate">{option.label}</span>
                    {option.hint ? (
                      <span className="text-muted-foreground shrink-0 text-xs">{option.hint}</span>
                    ) : null}
                  </button>
                )
              })
            )}
          </div>
          {value.length > 0 ? (
            <div className="mt-2 flex justify-end border-t pt-2">
              <Button type="button" variant="ghost" size="sm" onClick={() => onChange([])}>
                清空
              </Button>
            </div>
          ) : null}
        </PopoverContent>
      </PopoverPortal>
    </Popover>
  )
}

interface ComboboxProps {
  options: SelectOption[]
  value: string
  onChange: (next: string) => void
  placeholder?: string
  /** 允许清空（对应"留空 = 不限制/使用默认"） */
  clearable?: boolean
  /** 允许输入候选之外的自定义值（如 OAuth scope：可选择常用值，也可输入自定义名称） */
  allowCustom?: boolean
  disabled?: boolean
  className?: string
}

/** 下拉单选（可搜索）：用于证书、默认职务等单选资源引用 */
export function Combobox({
  options,
  value,
  onChange,
  placeholder,
  clearable = true,
  allowCustom = false,
  disabled,
  className,
}: ComboboxProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [keyword, setKeyword] = useState('')

  const filtered = useMemo(() => {
    if (!keyword.trim()) return options
    const lower = keyword.trim().toLowerCase()
    return options.filter((option) => option.label.toLowerCase().includes(lower))
  }, [options, keyword])

  const current = options.find((option) => option.value === value)
  // 自定义值：候选里没有时直接显示原始值，避免"选了却显示占位符"
  const currentLabel = current?.label ?? (allowCustom && value ? value : undefined)
  const custom = keyword.trim()
  const showCustom = allowCustom && custom !== '' && !options.some((option) => option.value === custom)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          role="combobox"
          disabled={disabled}
          className={cn(
            'border-input/70 bg-card flex h-10 w-full items-center justify-between gap-2 rounded-xl border px-3.5 text-left text-[13px] transition-[color,box-shadow,border-color] outline-none',
            'focus-visible:border-ring focus-visible:ring-ring/25 focus-visible:ring-4 disabled:cursor-not-allowed disabled:opacity-50',
            className,
          )}
        >
          <span className={cn('min-w-0 flex-1 truncate', !currentLabel && 'text-muted-foreground')}>
            {currentLabel ?? placeholder ?? '-'}
          </span>
          <ChevronDown className="size-4 shrink-0 opacity-60" />
        </button>
      </PopoverTrigger>
      {/* 必须 Portal：表单卡片是 surface overflow-hidden，内联浮层会被裁切、被后续卡片盖住 */}
      <PopoverPortal>
        <PopoverContent
          align="start"
          sideOffset={4}
          className="bg-popover text-popover-foreground border-border/70 shadow-pop z-50 w-[var(--radix-popover-trigger-width)] min-w-56 rounded-xl border p-2"
        >
          <Input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="搜索…"
            className="mb-2 h-8"
          />
          <div className="max-h-60 overflow-y-auto">
            {clearable ? (
              <button
                type="button"
                onClick={() => {
                  onChange('')
                  setOpen(false)
                }}
                className="hover:bg-accent text-muted-foreground w-full rounded-lg px-2.5 py-2 text-left text-[13px]"
              >
                {placeholder ?? '不设置'}
              </button>
            ) : null}
            {showCustom ? (
              <button
                type="button"
                onClick={() => {
                  onChange(custom)
                  setKeyword('')
                  setOpen(false)
                }}
                className="hover:bg-accent mb-0.5 flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[13px]"
              >
                <Plus className="size-3.5 shrink-0" />
                <span className="min-w-0 flex-1 truncate">
                  {t('common.useCustomValue', { value: custom })}
                </span>
              </button>
            ) : null}
            {filtered.map((option) => (
              <button
                key={option.value}
                type="button"
                onClick={() => {
                  onChange(option.value)
                  setOpen(false)
                }}
                className="hover:bg-accent flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[13px]"
              >
                <span className="min-w-0 flex-1 truncate">{option.label}</span>
                {option.hint ? <span className="text-muted-foreground text-xs">{option.hint}</span> : null}
                {value === option.value ? <Check className="size-4" /> : null}
              </button>
            ))}
          </div>
        </PopoverContent>
      </PopoverPortal>
    </Popover>
  )
}
