import { useTranslation } from 'react-i18next'
import { Label } from '@/shared/ui'

interface FormFieldProps {
  id: string
  labelKey: string
  hint?: string
  className?: string
  children: React.ReactNode
}

/** 表单字段（标签 + 控件 + 说明），供组织详情等页面复用 */
export function FormField({ id, labelKey, hint, className, children }: FormFieldProps) {
  const { t } = useTranslation()
  return (
    <div className={className ?? 'space-y-2'}>
      <Label htmlFor={id}>{t(labelKey)}</Label>
      {children}
      {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
    </div>
  )
}
