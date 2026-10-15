import { useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'
import { Input } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

interface TagInputProps {
  id?: string
  value: string[]
  onChange: (next: string[]) => void
  placeholder?: string
  disabled?: boolean
  className?: string
}

/** 一次可录入多个标签的分隔符（粘贴 "a, b、c" 时一次性拆成多个标签） */
const TAG_SEPARATORS = /[,，、\n]/

/**
 * 标签输入（chips）：已选标签以可删除的胶囊展示，输入 + 回车/逗号添加。
 *
 * 交互约定（对齐主流标签输入）：
 * - 回车 / 逗号 / 失焦 → 提交当前草稿（自动去重、去空白）；
 * - 草稿为空时按退格 → 删除最后一个标签；
 * - 支持直接粘贴多个（逗号 / 顿号 / 换行分隔）。
 */
export function TagInput({ id, value, onChange, placeholder, disabled, className }: TagInputProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState('')

  const commit = (raw: string) => {
    const parts = raw
      .split(TAG_SEPARATORS)
      .map((item) => item.trim())
      .filter(Boolean)
    setDraft('')
    if (parts.length === 0) return
    const next = [...value]
    for (const part of parts) {
      if (!next.includes(part)) next.push(part)
    }
    if (next.length !== value.length) onChange(next)
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' || event.key === ',' || event.key === '，') {
      event.preventDefault()
      commit(draft)
      return
    }
    if (event.key === 'Backspace' && draft === '' && value.length > 0) {
      onChange(value.slice(0, -1))
    }
  }

  return (
    <div
      className={cn(
        'border-input/70 bg-card flex min-h-10 w-full flex-wrap items-center gap-1.5 rounded-xl border px-2 py-1.5 transition-colors',
        'focus-within:border-ring focus-within:ring-ring/25 focus-within:ring-4',
        disabled && 'cursor-not-allowed opacity-60',
        className,
      )}
    >
      {value.map((tag) => (
        <span
          key={tag}
          className="bg-secondary text-secondary-foreground inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px]"
        >
          {tag}
          <button
            type="button"
            disabled={disabled}
            aria-label={t('common.removeTag', { tag })}
            className="hover:text-destructive disabled:pointer-events-none"
            onClick={() => onChange(value.filter((item) => item !== tag))}
          >
            <X className="size-3" aria-hidden="true" />
          </button>
        </span>
      ))}
      <Input
        id={id}
        value={draft}
        disabled={disabled}
        placeholder={value.length === 0 ? placeholder : ''}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={() => commit(draft)}
        className="h-6 min-w-24 flex-1 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0 dark:bg-transparent"
      />
    </div>
  )
}
