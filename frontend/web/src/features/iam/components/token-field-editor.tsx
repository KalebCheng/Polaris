import { useTranslation } from 'react-i18next'
import { Plus, Trash2 } from 'lucide-react'
import {
  Button,
  Combobox,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/shared/ui'
import type { TokenAttribute } from '@/shared/api/types'
import { tokenFieldOptions } from './application-form'

/** 行内字段的原生类型：部门类字段是数组，其余是字符串/布尔/数字。 */
function fieldType(field: string): TokenAttribute['type'] {
  return ['department_ids', 'departments', 'groups'].includes(field) ? 'array' : 'string'
}

/**
 * access_token 自定义 claim 编辑器，对齐 Casdoor 的 Token 属性：
 * 名称 / 类别（现有字段、静态值）/ 值 / 类型。
 */
export function TokenFieldEditor({
  value,
  onChange,
}: {
  value: TokenAttribute[]
  onChange: (next: TokenAttribute[]) => void
}) {
  const { t } = useTranslation()

  const update = (index: number, patch: Partial<TokenAttribute>) => {
    onChange(value.map((row, i) => (i === index ? { ...row, ...patch } : row)))
  }

  return (
    <div className="space-y-2">
      {value.length === 0 ? (
        <p className="text-muted-foreground text-xs leading-relaxed">{t('applications.tokenFieldsEmpty')}</p>
      ) : null}
      <div className="text-muted-foreground hidden gap-2 px-1 text-xs sm:grid sm:grid-cols-[minmax(0,1fr)_140px_minmax(0,1.2fr)_110px_auto]">
        <span>{t('applications.tokenAttributeName')}</span>
        <span>{t('applications.tokenAttributeCategory')}</span>
        <span>{t('applications.tokenAttributeValue')}</span>
        <span>{t('applications.tokenAttributeType')}</span>
        <span className="w-9" />
      </div>
      {value.map((row, index) => (
        <div
          key={index}
          className="grid items-center gap-2 rounded-lg border px-3 py-2 sm:grid-cols-[minmax(0,1fr)_140px_minmax(0,1.2fr)_110px_auto]"
        >
          <Input
            value={row.name}
            onChange={(event) => update(index, { name: event.target.value })}
            placeholder={t('applications.tokenAttributeNamePlaceholder')}
          />
          <Select
            value={row.category}
            onValueChange={(next) =>
              update(index, {
                category: next as TokenAttribute['category'],
                value: '',
                type: 'string',
              })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="existing">{t('applications.tokenCategoryExisting')}</SelectItem>
              <SelectItem value="static">{t('applications.tokenCategoryStatic')}</SelectItem>
            </SelectContent>
          </Select>
          {row.category === 'static' ? (
            <Input
              value={row.value}
              onChange={(event) => update(index, { value: event.target.value })}
              placeholder={t('applications.tokenAttributeValuePlaceholder')}
            />
          ) : (
            <Combobox
              options={tokenFieldOptions}
              value={row.value}
              onChange={(next) =>
                update(index, {
                  value: next,
                  name: row.name || next,
                  type: fieldType(next),
                })
              }
              placeholder={t('applications.tokenFieldPlaceholder')}
            />
          )}
          <Select
            value={row.type}
            onValueChange={(next) => update(index, { type: next as TokenAttribute['type'] })}
            disabled={row.category === 'existing'}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="string">{t('applications.tokenTypeString')}</SelectItem>
              <SelectItem value="array">{t('applications.tokenTypeArray')}</SelectItem>
            </SelectContent>
          </Select>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('applications.removeTokenField')}
            onClick={() => onChange(value.filter((_, i) => i !== index))}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => onChange([...value, { name: '', category: 'existing', value: '', type: 'string' }])}
      >
        <Plus className="size-4" />
        {t('applications.addTokenField')}
      </Button>
    </div>
  )
}
