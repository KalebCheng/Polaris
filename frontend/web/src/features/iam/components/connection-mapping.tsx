import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Info, Plus, X } from 'lucide-react'
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
} from '@/shared/ui'
import type { ConnectionSourceField, ConnectionTargetField } from '@/shared/api/types'
import {
  decodeMapping,
  emptyRule,
  encodeMapping,
  type MappingRule,
} from './connection-mapping-codec'

interface ConnectionMappingProps {
  targetFields: ConnectionTargetField[]
  sourceFields: ConnectionSourceField[]
  /** 已存的映射（后端编码串） */
  value: Record<string, string>
  onChange: (next: Record<string, string>) => void
}

/**
 * 字段映射（交互取向对齐成熟产品，但按 Polaris 的排版语言重做）：
 * 每个本地字段**一行**——左侧字段名与技术名，右侧「映射规则 + 字段值 + 未映射开关 + 添加备选」。
 * 窄屏自动换行；控件固定宽度 + 不换行，避免出现竖排文字。
 * 源字段下拉同时显示**名称与源字段键**（如 `手机号码 · mobile`），与外部系统的字段一一对应。
 */
export function ConnectionMapping({ targetFields, sourceFields, value, onChange }: ConnectionMappingProps) {
  const { t } = useTranslation()
  const [rules, setRules] = useState<Record<string, MappingRule>>(() => decodeMapping(value))

  const rows = useMemo(
    () => targetFields.filter((field) => field.group === 'account'),
    [targetFields],
  )
  const sources = useMemo(
    () => sourceFields.filter((field) => field.group === 'account'),
    [sourceFields],
  )

  const commit = (next: Record<string, MappingRule>) => {
    setRules(next)
    onChange(encodeMapping(next))
  }
  const update = (key: string, patch: Partial<MappingRule>) => {
    const current = rules[key] ?? emptyRule()
    commit({ ...rules, [key]: { ...current, ...patch } })
  }
  const setFieldAt = (key: string, index: number, field: string) => {
    const current = rules[key] ?? emptyRule()
    const fields = [...current.fields]
    fields[index] = field
    update(key, { fields })
  }
  const addFallback = (key: string) => {
    const current = rules[key] ?? emptyRule()
    update(key, { fields: [...current.fields, ''] })
  }
  const removeFallback = (key: string, index: number) => {
    const current = rules[key] ?? emptyRule()
    update(key, { fields: current.fields.filter((_, i) => i !== index) })
  }
  const reset = (key: string) => {
    const next = { ...rules }
    delete next[key]
    commit(next)
  }

  /** 源字段选项：名称 + 源字段键（mono），样例值放在 title 里供悬停查看 */
  const sourceItems = sources.map((source) => (
    <SelectItem key={source.key} value={source.key} title={source.sample ? `示例：${source.sample}` : undefined}>
      <span className="flex items-baseline gap-2">
        <span>{source.label}</span>
        <span className="text-muted-foreground font-mono text-[11px]">{source.key}</span>
      </span>
    </SelectItem>
  ))

  return (
    <div className="space-y-3">
      <p className="text-muted-foreground text-xs">{t('connections.mappingHint')}</p>

      <div className="divide-border/60 divide-y rounded-2xl border border-border/70 shadow-soft">
        {rows.map((field) => {
          const rule = rules[field.key] ?? emptyRule()
          const unmapped = rule.unmapped
          const mapped = !unmapped && encodeMapping({ [field.key]: rule })[field.key] !== undefined
          return (
            <div key={field.key} className="flex flex-wrap items-start gap-x-6 gap-y-3 p-4">
              {/* 本地字段 */}
              <div className="w-52 shrink-0 space-y-0.5">
                <p className="text-[13px] font-medium">
                  {field.required ? <span className="text-destructive">* </span> : null}
                  {field.label}
                </p>
                <p className="text-muted-foreground font-mono text-[11px]">{field.display ?? field.key}</p>
                {field.hint ? (
                  <p className="text-muted-foreground flex items-start gap-1 text-[11px]">
                    <Info className="mt-0.5 size-3.5 shrink-0" />
                    <span>{field.hint}</span>
                  </p>
                ) : null}
              </div>

              {/* 映射规则 + 字段值 */}
              <div className="flex min-w-0 flex-1 flex-wrap items-center gap-3">
                <Select
                  value={rule.mode}
                  disabled={unmapped}
                  onValueChange={(mode) => update(field.key, { mode: mode as MappingRule['mode'] })}
                >
                  <SelectTrigger className="h-9 w-32 shrink-0" aria-label={t('connections.mappingRule')}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="field">{t('connections.mappingRuleField')}</SelectItem>
                    <SelectItem value="constant">{t('connections.mappingRuleConstant')}</SelectItem>
                  </SelectContent>
                </Select>

                {rule.mode === 'constant' ? (
                  <Input
                    className="h-9 min-w-56 flex-1"
                    disabled={unmapped}
                    value={rule.constant}
                    placeholder={t('connections.mappingConstantPlaceholder')}
                    onChange={(event) => update(field.key, { constant: event.target.value })}
                  />
                ) : (
                  <div className="flex min-w-56 flex-1 flex-col gap-2">
                    {rule.fields.map((selected, index) => (
                      <div key={index} className="flex items-center gap-2">
                        <Select
                          value={selected}
                          disabled={unmapped}
                          onValueChange={(source) => setFieldAt(field.key, index, source)}
                        >
                          <SelectTrigger className="h-9 min-w-0 flex-1">
                            <SelectValue placeholder={t('connections.mappingPick')} />
                          </SelectTrigger>
                          <SelectContent className="max-h-72">{sourceItems}</SelectContent>
                        </Select>
                        {index > 0 ? (
                          <Button
                            type="button"
                            size="icon"
                            variant="ghost"
                            aria-label={t('connections.mappingRemoveFallback')}
                            onClick={() => removeFallback(field.key, index)}
                          >
                            <X className="size-4" />
                          </Button>
                        ) : null}
                      </div>
                    ))}
                    {rule.fields.length > 1 ? (
                      <p className="text-muted-foreground text-[11px]">{t('connections.mappingFallbackHint')}</p>
                    ) : null}
                  </div>
                )}
              </div>

              {/* 映射状态与操作 */}
              <div className="flex shrink-0 items-center gap-4">
                <label className="flex cursor-pointer items-center gap-2 whitespace-nowrap">
                  <Switch
                    checked={unmapped}
                    disabled={false}
                    onCheckedChange={(checked) => update(field.key, { unmapped: checked })}
                  />
                  <span className="text-muted-foreground text-[11px]">
                    {mapped ? t('connections.mappingMapped') : t('connections.mappingUnmapped')}
                  </span>
                </label>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="whitespace-nowrap"
                  disabled={unmapped || rule.mode === 'constant'}
                  onClick={() => addFallback(field.key)}
                >
                  <Plus className="size-3.5" /> {t('connections.mappingAdd')}
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button type="button" size="icon" variant="ghost" aria-label={t('connections.mappingMore')}>
                      ⋯
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onSelect={() => reset(field.key)}>
                      {t('connections.mappingClear')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
          )
        })}
      </div>

      <p className="text-muted-foreground text-xs">{t('connections.mappingDefaultHint')}</p>
    </div>
  )
}
