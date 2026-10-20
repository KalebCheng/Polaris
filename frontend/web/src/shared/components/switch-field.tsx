import { useTranslation } from 'react-i18next'
import { Switch } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

interface SwitchFieldProps {
  /** i18n key：开关名称 */
  labelKey: string
  /** i18n key：一句话说明（告诉用户开启后的后果） */
  descriptionKey?: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  disabled?: boolean
  className?: string
}

/**
 * 开关行：名称 + 说明 + 右侧开关，整行可点。
 * 用于「新建 / 编辑」页的布尔策略项，避免裸 Switch 与标签错位、点击区域过小。
 */
export function SwitchField({
  labelKey,
  descriptionKey,
  checked,
  onCheckedChange,
  disabled,
  className,
}: SwitchFieldProps) {
  const { t } = useTranslation()
  return (
    <label
      className={cn(
        'border-border/70 flex items-start justify-between gap-4 rounded-xl border px-3.5 py-2.5 transition-colors',
        disabled
          ? 'cursor-not-allowed opacity-60'
          : 'hover:border-primary/25 hover:bg-accent/40 cursor-pointer',
        className,
      )}
    >
      <span className="min-w-0 space-y-0.5">
        <span className="block text-[13px] font-medium">{t(labelKey)}</span>
        {descriptionKey ? (
          <span className="text-muted-foreground block text-xs leading-relaxed">{t(descriptionKey)}</span>
        ) : null}
      </span>
      <Switch className="mt-0.5" checked={checked} disabled={disabled} onCheckedChange={onCheckedChange} />
    </label>
  )
}
