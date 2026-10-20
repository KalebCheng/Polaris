import { useTranslation } from 'react-i18next'
import { Label } from '@/shared/ui'

interface FieldProps {
  id: string
  labelKey: string
  hint?: string
  className?: string
  children: React.ReactNode
}

/** 表单字段容器：统一标签、说明与控件间距 */
export function Field({ id, labelKey, hint, className, children }: FieldProps) {
  const { t } = useTranslation()
  return (
    <div className={className ?? 'space-y-2'}>
      <Label htmlFor={id}>{t(labelKey)}</Label>
      {children}
      {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
    </div>
  )
}
