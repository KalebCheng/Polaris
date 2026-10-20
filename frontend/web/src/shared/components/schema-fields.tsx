import { useTranslation } from 'react-i18next'
import {
  Input,
  Label,
  MultiSelect,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
  Switch,
  Textarea,
} from '@/shared/ui'
import { TagInput } from '@/shared/components/tag-input'
import i18n from '@/shared/i18n'
import { cn } from '@/shared/lib/utils'

/**
 * 目录 schema 渲染器：把「后端声明的字段 schema」渲染成表单控件。
 *
 * 身份连接（connections）与提供商（providers）共用这一份实现——两者的字段来源都是后端目录：
 *   - 字段标签/提示优先取 i18n（`<prefix>.field.<key>` / `<prefix>.field.<key>Hint`），
 *     缺词条时回退目录里的中文 label/hint，因此"界面出现原始 i18n key"不会再发生；
 *   - 新增一个类型只需在后端登记字段，前端无需改动（杜绝三处硬编码类型字段）。
 */

/** 表单值：文本/数字/开关/多值标签（组织选择用数字字符串表示部门 id） */
export type FieldValueMap = Record<string, string | boolean | string[]>

/** 与后端 catalog 字段一一对应的结构类型（连接与提供商共用） */
export interface SchemaField {
  key: string
  label: string
  group?: string
  type: string
  required?: boolean
  /** 创建后不可修改：编辑态禁止修改（后端也按标识定位，改了也不会生效） */
  immutable?: boolean
  secret?: boolean
  policy?: boolean
  placeholder?: string
  hint?: string
  options?: { value: string; label: string; group?: string }[]
  /** 同组字段并排展示（桌面端两列），如账号匹配的两个下拉 */
  row_group?: string
  /** 动态选项来源（由页面解析为 dynamicOptions 后传入） */
  options_source?: string
  default?: string
  show_when?: { field: string; equals: string }
  show_when_mode?: string
}

function visible(field: SchemaField, values: FieldValueMap): boolean {
  if (field.show_when_mode) {
    return String(values['schedule_mode'] ?? '') === field.show_when_mode
  }
  if (!field.show_when) return true
  return String(values[field.show_when.field] ?? '') === field.show_when.equals
}

/**
 * 词条查找顺序：**类型专属 → 通用 → 目录声明**。
 *
 * 为什么要类型维度：同一个列名在不同类型里含义不同——`endpoint` 对本地存储是"存储根目录"、
 * 对 S3 是"端点"、对短信网关是"网关地址"。若只写一条通用的 `providers.field.endpoint`，
 * 界面就会出现「端点 / 根目录 / 网关地址」这种把三种含义硬拼在一起的标签（用户直接看不懂）。
 * 目录（后端 catalog）本就按类型声明了各自的中文名，故：
 *   - `providers.field.<type>.<key>`：类型专属（如 `providers.field.local.endpoint`）
 *   - `providers.field.<key>`：该含义在多数类型下通用时使用
 *   - `field.label`：目录声明（中文兜底）
 */
function label(
  field: SchemaField,
  prefix: string,
  typeKey: string | undefined,
  t: (key: string) => string,
): string {
  if (typeKey) {
    const scoped = `${prefix}.field.${typeKey}.${field.key}`
    if (i18n.exists(scoped)) return t(scoped)
  }
  const key = `${prefix}.field.${field.key}`
  return i18n.exists(key) ? t(key) : field.label
}

function hint(
  field: SchemaField,
  prefix: string,
  typeKey: string | undefined,
  t: (key: string) => string,
): string | undefined {
  if (typeKey) {
    const scoped = `${prefix}.field.${typeKey}.${field.key}Hint`
    if (i18n.exists(scoped)) return t(scoped)
  }
  const key = `${prefix}.field.${field.key}Hint`
  if (i18n.exists(key)) return t(key)
  return field.hint
}

type SchemaOption = { value: string; label: string; group?: string }

/** 按目录声明的 group 切分下拉选项；未分组选项保持原有顺序。 */
function groupOptions(options: SchemaOption[]): { label: string; options: SchemaOption[] }[] {
  const groups: { label: string; options: SchemaOption[] }[] = []
  const indexes = new Map<string, number>()
  for (const option of options) {
    const groupLabel = option.group ?? ''
    const index = indexes.get(groupLabel)
    if (index !== undefined) {
      groups[index].options.push(option)
      continue
    }
    indexes.set(groupLabel, groups.length)
    groups.push({ label: groupLabel, options: [option] })
  }
  return groups
}

interface SchemaFieldsProps {
  fields: SchemaField[]
  values: FieldValueMap
  onChange: (key: string, value: string | boolean | string[]) => void
  /** i18n 前缀：connections | providers */
  i18nPrefix: string
  /** 当前类型 key：用于优先取该类型的专属词条（如 providers.field.local.endpoint） */
  typeKey?: string
  /** 编辑态：敏感字段已配置时提示「留空表示不修改」 */
  editing?: boolean
  /** 组织下拉的可选项（department 控件用） */
  departments?: { id: number; name: string; display_name: string }[]
  /** 按字段 key 提供动态下拉选项（如短信提供商 / MFA 模板） */
  dynamicOptions?: Record<string, { value: string; label: string }[]>
}

export function SchemaFields({
  fields,
  values,
  onChange,
  i18nPrefix,
  typeKey,
  editing = false,
  departments = [],
  dynamicOptions = {},
}: SchemaFieldsProps) {
  const { t } = useTranslation()
  return (
    <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
      {fields
        .filter((field) => visible(field, values))
        .map((field, index) => {
          const id = `${i18nPrefix}-field-${field.key}`
          const text = hint(field, i18nPrefix, typeKey, t)
          const placeholder =
            field.secret && editing
              ? t('common.secretKeep')
              : field.placeholder
                ? field.placeholder
                : undefined
          if (field.type === 'multiselect') {
            return (
              <div
                key={`${field.key}-${index}`}
                data-schema-row={field.row_group ?? ''}
                className={cn('space-y-2', !field.row_group && 'sm:col-span-2')}
              >
                <Label htmlFor={id}>
                  {label(field, i18nPrefix, typeKey, t)}
                  {field.required ? <span className="text-destructive"> *</span> : null}
                </Label>
                <MultiSelect
                  options={(field.options ?? []).map((option) => ({
                    value: option.value,
                    label: option.label,
                  }))}
                  value={Array.isArray(values[field.key]) ? (values[field.key] as string[]) : []}
                  onChange={(next) => onChange(field.key, next)}
                  placeholder={field.placeholder ?? t('common.selectPlaceholder')}
                />
                {text ? <p className="text-muted-foreground text-xs">{text}</p> : null}
              </div>
            )
          }
          if (field.type === 'tags') {
            return (
              <div
                key={`${field.key}-${index}`}
                data-schema-row={field.row_group ?? ''}
                className={cn('space-y-2', !field.row_group && 'sm:col-span-2')}
              >
                <Label htmlFor={id}>
                  {label(field, i18nPrefix, typeKey, t)}
                  {field.required ? <span className="text-destructive"> *</span> : null}
                </Label>
                <TagInput
                  value={Array.isArray(values[field.key]) ? (values[field.key] as string[]) : []}
                  onChange={(next) => onChange(field.key, next)}
                  placeholder={field.placeholder ?? t('common.tagsPlaceholder')}
                />
                {text ? <p className="text-muted-foreground text-xs">{text}</p> : null}
              </div>
            )
          }
          if (field.type === 'department') {
            return (
              <div
                key={`${field.key}-${index}`}
                data-schema-row={field.row_group ?? ''}
                className={cn('space-y-2', !field.row_group && 'sm:col-span-2')}
              >
                <Label htmlFor={id}>{label(field, i18nPrefix, typeKey, t)}</Label>
                <Select
                  value={String(values[field.key] ?? '')}
                  onValueChange={(value) => onChange(field.key, value)}
                >
                  <SelectTrigger id={id}>
                    <SelectValue placeholder={t('common.departmentRoot')} />
                  </SelectTrigger>
                  <SelectContent>
                    {departments.map((dept) => (
                      <SelectItem key={dept.id} value={String(dept.id)}>
                        {dept.display_name || dept.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {text ? <p className="text-muted-foreground text-xs">{text}</p> : null}
              </div>
            )
          }
          if (field.type === 'switch') {
            return (
              <div
                key={`${field.key}-${index}`}
                data-schema-row={field.row_group ?? ''}
                className={cn('flex items-start gap-3', !field.row_group && 'sm:col-span-2')}
              >
                <Switch
                  id={id}
                  checked={values[field.key] === true}
                  onCheckedChange={(checked) => onChange(field.key, checked)}
                />
                <div className="space-y-0.5">
                  <Label htmlFor={id}>{label(field, i18nPrefix, typeKey, t)}</Label>
                  {text ? <p className="text-muted-foreground text-xs">{text}</p> : null}
                </div>
              </div>
            )
          }
          if (field.type === 'textarea') {
            return (
              <div
                key={`${field.key}-${index}`}
                data-schema-row={field.row_group ?? ''}
                className={cn('space-y-2', !field.row_group && 'sm:col-span-2')}
              >
                <Label htmlFor={id}>
                  {label(field, i18nPrefix, typeKey, t)}
                  {field.required ? <span className="text-destructive"> *</span> : null}
                </Label>
                <Textarea
                  id={id}
                  value={String(values[field.key] ?? '')}
                  placeholder={placeholder}
                  rows={5}
                  onChange={(event) => onChange(field.key, event.target.value)}
                />
                {text ? <p className="text-muted-foreground text-xs">{text}</p> : null}
              </div>
            )
          }
          return (
            <div
              key={`${field.key}-${index}`}
              data-schema-row={field.row_group ?? ''}
              className={cn('space-y-2', !field.row_group && 'sm:col-span-2')}
            >
              <Label htmlFor={id}>
                {label(field, i18nPrefix, typeKey, t)}
                {field.required ? <span className="text-destructive"> *</span> : null}
              </Label>
              {field.type === 'select' ? (
                <Select
                  value={String(values[field.key] ?? field.default ?? '')}
                  onValueChange={(value) => onChange(field.key, value)}
                  disabled={editing && field.immutable}
                >
                  <SelectTrigger id={id}>
                    <SelectValue placeholder={placeholder ?? t('common.selectPlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {groupOptions(
                      field.options && field.options.length > 0
                        ? field.options
                        : (dynamicOptions[field.key] ?? []),
                    ).map((group) => (
                      <SelectGroup key={group.label || '__ungrouped'}>
                        {group.label ? <SelectLabel>{group.label}</SelectLabel> : null}
                        {group.options.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  id={id}
                  type={field.type === 'password' ? 'password' : field.type === 'number' ? 'number' : 'text'}
                  value={String(values[field.key] ?? '')}
                  placeholder={placeholder}
                  autoComplete={field.secret ? 'new-password' : undefined}
                  disabled={editing && field.immutable}
                  onChange={(event) => onChange(field.key, event.target.value)}
                />
              )}
              {text ? <p className="text-muted-foreground text-xs">{text}</p> : null}
            </div>
          )
        })}
    </div>
  )
}
