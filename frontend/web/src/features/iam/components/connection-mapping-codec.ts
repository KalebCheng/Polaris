/**
 * 字段映射的取值编码（与后端 `service/syncer/mapping.go` 一一对应）：
 *   "mobile"        → 取源字段 mobile
 *   "mobile|email"  → 备选源字段，取首个非空（界面上的「添加」）
 *   "=远程"         → 固定值（界面上的「映射规则 → 固定值」）
 * 历史数据是无前缀单值，因此解析向后兼容。
 */
export type MappingMode = 'field' | 'constant'

export interface MappingRule {
  /** 映射规则：选择字段 / 固定值 */
  mode: MappingMode
  /** 源字段列表：第 1 个为主字段，其余为备选 */
  fields: string[]
  /** 固定值（mode=constant 时生效） */
  constant: string
  /** 未映射：该行不参与映射（界面上的「未映射」开关） */
  unmapped: boolean
}

export function emptyRule(): MappingRule {
  return { mode: 'field', fields: [''], constant: '', unmapped: false }
}

/** 解析单行规则串 */
export function decodeRule(spec: string | undefined): MappingRule {
  if (!spec) return emptyRule()
  if (spec.startsWith('=')) {
    return { mode: 'constant', fields: [''], constant: spec.slice(1), unmapped: false }
  }
  const fields = spec
    .split('|')
    .map((item) => item.trim())
    .filter(Boolean)
  return { mode: 'field', fields: fields.length ? fields : [''], constant: '', unmapped: false }
}

/** 编码单行规则串；返回空串表示该行不落库（未映射或未填） */
export function encodeRule(rule: MappingRule): string {
  if (rule.unmapped) return ''
  if (rule.mode === 'constant') {
    const constant = rule.constant.trim()
    return constant ? `=${constant}` : ''
  }
  return rule.fields
    .map((field) => field.trim())
    .filter(Boolean)
    .join('|')
}

export function decodeMapping(mapping: Record<string, string> | undefined): Record<string, MappingRule> {
  const out: Record<string, MappingRule> = {}
  for (const [key, spec] of Object.entries(mapping ?? {})) {
    out[key] = decodeRule(spec)
  }
  return out
}

export function encodeMapping(rules: Record<string, MappingRule>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, rule] of Object.entries(rules)) {
    const spec = encodeRule(rule)
    if (spec) out[key] = spec
  }
  return out
}

/** Merge one entity mapping without touching the other mapping contracts. */
export function mergeMappingSection(
  value: Record<string, string>,
  groupKeys: string[],
  rules: Record<string, MappingRule>,
): Record<string, string> {
  const keys = new Set(groupKeys)
  const merged = Object.fromEntries(Object.entries(value).filter(([key]) => !keys.has(key)))
  return { ...merged, ...encodeMapping(rules) }
}
