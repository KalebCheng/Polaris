import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  type SelectOption,
} from '@/shared/ui'
import type { Provider } from '@/shared/api/types'
import { providerApi } from '../api'
import { Field } from './field'
import type { SigninMethodEntry } from './application-form'

/** 分段选项（用于类型 / 平台这类少量互斥选项，比下拉更直观） */
export function OptionChips({
  options,
  value,
  onChange,
}: {
  options: SelectOption[]
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {options.map((option) => (
        <Button
          key={option.value}
          type="button"
          size="sm"
          variant={option.value === value ? 'default' : 'outline'}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </Button>
      ))}
    </div>
  )
}

export function NotificationTemplateFields({
  entry,
  providers,
  ttl,
  onTtlChange,
  update,
}: {
  entry: SigninMethodEntry
  providers: Provider[]
  ttl: number
  onTtlChange: (value: number) => void
  update: (patch: Partial<SigninMethodEntry>) => void
}) {
  const { t } = useTranslation()
  const category = entry.kind === 'SMSCode' ? 'sms' : 'email'
  const scene = entry.kind === 'MagicLink' ? 'magic_link' : 'login_code'
  const options = providers.filter(
    (p) => (p.category ?? '').toLowerCase() === category && p.state === 'active',
  )
  const [templates, setTemplates] = useState<{ id: number; name: string; is_default: boolean }[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    let cancelled = false
    if (!entry.notifyProviderId) {
      setTemplates([])
      return
    }
    setLoading(true)
    void providerApi
      .templates(entry.notifyProviderId, scene)
      .then((data) => {
        if (cancelled) return
        const items = data.items ?? []
        setTemplates(items)
        const current = items.find((item) => item.id === entry.notifyTemplateId)
        if (!current && items.length > 0) {
          const preferred = items.find((item) => item.is_default) ?? items[0]
          update({ notifyTemplateId: preferred.id })
        }
      })
      .catch(() => {
        if (!cancelled) setTemplates([])
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [entry.notifyProviderId, entry.notifyTemplateId, scene, update])

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field
        id={`method-${entry.id}-provider`}
        labelKey="applications.notificationProvider"
        hint={t('applications.notificationProviderHint')}
      >
        <Select
          value={entry.notifyProviderId ? String(entry.notifyProviderId) : ''}
          onValueChange={(value) => update({ notifyProviderId: Number(value), notifyTemplateId: undefined })}
        >
          <SelectTrigger id={`method-${entry.id}-provider`}>
            <SelectValue placeholder={t('common.selectPlaceholder')} />
          </SelectTrigger>
          <SelectContent>
            {options.map((provider) => (
              <SelectItem key={provider.id} value={String(provider.id)}>
                {provider.display_name || provider.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <Field
        id={`method-${entry.id}-template`}
        labelKey="applications.notificationTemplate"
        hint={t('applications.notificationTemplateHint')}
      >
        <Select
          value={entry.notifyTemplateId ? String(entry.notifyTemplateId) : ''}
          onValueChange={(value) => update({ notifyTemplateId: Number(value) })}
          disabled={!entry.notifyProviderId || loading || templates.length === 0}
        >
          <SelectTrigger id={`method-${entry.id}-template`}>
            <SelectValue placeholder={t('applications.notificationTemplatePlaceholder')} />
          </SelectTrigger>
          <SelectContent>
            {templates.map((template) => (
              <SelectItem key={template.id} value={String(template.id)}>
                {template.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <Field
        id={`method-${entry.id}-ttl`}
        labelKey="applications.codeTTL"
        hint={t('applications.codeTTLHint')}
      >
        <Input
          id={`method-${entry.id}-ttl`}
          type="number"
          min={60}
          value={ttl}
          onChange={(event) => onTtlChange(Number(event.target.value))}
        />
      </Field>
    </div>
  )
}
