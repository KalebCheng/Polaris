import { useTranslation } from 'react-i18next'
import { Plus, X } from 'lucide-react'
import { Button, Input } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

interface RepeatableListProps {
  /** 输入框占位符（如 https://app.example.com/oauth/callback） */
  placeholder?: string
  value: string[]
  onChange: (next: string[]) => void
  /** 单行校验：返回错误文案（i18n key 或已翻译文本）表示不合法；返回 undefined 表示通过 */
  validate?: (value: string) => string | undefined
  /** 最少保留几行（默认 1 行，保证始终有可编辑的输入框） */
  minRows?: number
  disabled?: boolean
  className?: string
}

/**
 * 多值列表编辑器：一行一个值，可增删。
 * 用于回调地址这类"可填多个"的字段——比多行文本框更明确，也更容易就地校验与去重。
 */
export function RepeatableList({
  placeholder,
  value,
  onChange,
  validate,
  minRows = 1,
  disabled,
  className,
}: RepeatableListProps) {
  const { t } = useTranslation()
  const rows = value.length > 0 ? value : ['']

  const update = (index: number, next: string) => {
    const copy = [...rows]
    copy[index] = next
    onChange(copy)
  }

  const remove = (index: number) => {
    const copy = rows.filter((_, i) => i !== index)
    onChange(copy.length >= minRows ? copy : [...copy, ''])
  }

  return (
    <div className={cn('space-y-2', className)}>
      {rows.map((row, index) => {
        const error = row.trim() ? validate?.(row) : undefined
        return (
          <div key={index} className="space-y-1">
            <div className="flex items-center gap-2">
              <Input
                value={row}
                disabled={disabled}
                onChange={(event) => update(index, event.target.value)}
                placeholder={placeholder}
                aria-invalid={Boolean(error)}
                aria-label={t('common.listItem', { index: index + 1 })}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                disabled={disabled}
                aria-label={t('common.removeItem')}
                onClick={() => remove(index)}
              >
                <X className="size-4" />
              </Button>
            </div>
            {error ? <p className="text-destructive text-xs">{error}</p> : null}
          </div>
        )
      })}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={() => onChange([...rows, ''])}
      >
        <Plus className="size-4" />
        {t('common.addItem')}
      </Button>
    </div>
  )
}
