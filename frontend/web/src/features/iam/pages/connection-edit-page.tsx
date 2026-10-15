import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowLeft } from 'lucide-react'
import { Button, Skeleton } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { FormWizard, type WizardStep } from '@/shared/components/form-wizard'
import i18n from '@/shared/i18n'
import { ConnectionTile } from '@/shared/components/connection-icon'
import type {
  ConnectionCatalog,
  ConnectionDirection,
  ConnectionField,
  ConnectionTypeInfo,
  Connector,
  Department,
  Syncer,
} from '@/shared/api/types'
import { connectionApi, connectorApi, departmentApi, syncerApi, type ConnectorPayload, type SyncerPayload } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { useMutation } from '../hooks/use-mutation'
import { ConnectionFields, type FieldValueMap } from '../components/connection-fields'
import { ConnectionMapping } from '../components/connection-mapping'
import {
  ConnectionCatalogGrid,
  toCatalogItems,
  type ConnectionCatalogItem,
} from '../components/connection-catalog'

type FieldValue = string | boolean | string[]

/**
 * 表单字段全部来自后端目录（`field.group` 决定归属哪一步）：
 *   basic → 基本信息；connection → 连接配置；scope → 同步范围与计划。
 * 这样"字段出现在哪一步"由后端单一来源决定，不会再出现"标识/名称在两步里重复出现"。
 */
const TAG_FIELD_KEYS = new Set(['name', 'display_name', 'is_enabled'])

/** 入方向的同步周期：仍是"间隔秒数"语义（定时校验的每天时刻/Cron 随下一批落地时移入目录） */
function scheduleField(options: { value: string; label: string }[]): ConnectionField {
  return { key: 'schedule', label: '同步周期', type: 'select', group: 'scope', options }
}

const NAME_PATTERN = /^[a-z0-9][a-z0-9-_]{1,30}$/

/**
 * 身份连接 新建 / 编辑页（**整页向导**，与「新增应用」一致，不使用弹窗）。
 * 路由：/institutions/:inst/connections/new 与 /institutions/:inst/connections/:id
 *
 * 类型来自目录（后端单一来源）；字段按 catalog 的 schema 渲染——同一屏只出现当前类型的字段。
 */
export default function ConnectionEditPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()
  const navigate = useNavigate()
  const { id = '' } = useParams()
  const [params] = useSearchParams()
  const isCreate = !id || id === 'new'
  const connectionID = Number(id)

  const direction: ConnectionDirection = params.get('direction') === 'out' ? 'out' : 'in'
  const [typeKey, setTypeKey] = useState(params.get('type') ?? '')
  const [subType, setSubType] = useState(params.get('sub_type') ?? '')

  const [catalog, setCatalog] = useState<ConnectionCatalog | null>(null)
  const [step, setStep] = useState(0)
  const [values, setValues] = useState<FieldValueMap>({})
  const [departments, setDepartments] = useState<Department[]>([])
  const [mapping, setMapping] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(!isCreate)
  // record 编辑态加载到的实例（类型由它定位，见下方两个 effect）
  const [record, setRecord] = useState<Syncer | Connector | null>(null)
  const { pending, run } = useMutation()

  const backToList = `/institutions/${institution}/connections?direction=${direction}`

  // 目录：类型与字段 schema 的唯一来源
  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const data = await connectionApi.catalog()
        if (!cancelled) setCatalog(data)
      } catch {
        if (!cancelled) setCatalog(null)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  // 同步目标：组织（部门树）下拉的可选项
  useEffect(() => {
    if (!institution) return
    let cancelled = false
    void (async () => {
      try {
        const list = await departmentApi.list({ institution, size: 200 })
        if (!cancelled) setDepartments(list.items ?? [])
      } catch {
        if (!cancelled) setDepartments([])
      }
    })()
    return () => {
      cancelled = true
    }
  }, [institution])

  const typeInfo: ConnectionTypeInfo | undefined = useMemo(() => {
    const types = catalog?.directions.find((group) => group.direction === direction)?.types ?? []
    return types.find((item) => item.key === typeKey)
  }, [catalog, direction, typeKey])

  // 必须保持数组引用稳定：`typeInfo` 未就绪时若每次渲染都新建 `[]`，
  // 下方的 applyDefaults 副作用会与重新渲染互相触发，形成渲染死循环。
  const basicFields = useMemo(
    () => (typeInfo?.fields ?? []).filter((field) => field.group === 'basic' || TAG_FIELD_KEYS.has(field.key)),
    [typeInfo],
  )
  const configFields = useMemo(
    () => (typeInfo?.fields ?? []).filter((field) => field.group === 'connection'),
    [typeInfo],
  )
  const scopeFields = useMemo(() => {
    const fromCatalog = (typeInfo?.fields ?? []).filter((field) => field.group === 'scope')
    if (direction !== 'in') return fromCatalog
    // 当前后端按"间隔秒数"生效，故只暴露真实生效的档位
    return [
      ...fromCatalog,
      scheduleField([
        { value: '', label: t('connections.scheduleNone') },
        { value: '1800', label: t('connections.schedule30m') },
        { value: '3600', label: t('connections.schedule1h') },
        { value: '21600', label: t('connections.schedule6h') },
        { value: '86400', label: t('connections.schedule1d') },
      ]),
    ]
  }, [typeInfo, direction, t])

  // 默认值：通用字段 + 类型字段的 default
  const applyDefaults = useCallback((fields: ConnectionField[]) => {
    setValues((prev) => {
      const next = { ...prev }
      for (const field of fields) {
        if (next[field.key] !== undefined) continue
        if (field.type === 'switch') next[field.key] = field.default === 'true'
        else if (field.type === 'tags') next[field.key] = []
        else next[field.key] = field.default ?? ''
      }
      if (next['name'] === undefined) next['name'] = ''
      return next
    })
  }, [])

  useEffect(() => {
    applyDefaults([...basicFields, ...configFields, ...scopeFields])
  }, [applyDefaults, basicFields, configFields, scopeFields])

  // 编辑态（第一步）：按 id 取记录。
  // **必须先用记录里的 type/sub_type 定位类型**——编辑地址只带 id（与 direction），
  // 不像新建那样在 URL 里带 type；若先等 typeInfo 再取记录会死锁，永远停在类型选择页。
  useEffect(() => {
    if (isCreate || connectionID <= 0 || !institution) return
    let cancelled = false
    setLoading(true)
    void (async () => {
      try {
        const data: Syncer | Connector =
          direction === 'in'
            ? await syncerApi.get(connectionID, institution)
            : await connectorApi.get(connectionID, institution)
        if (cancelled) return
        const raw = data as unknown as Record<string, unknown>
        setTypeKey(String(raw['type'] ?? ''))
        setSubType(String(raw['sub_type'] ?? ''))
        setRecord(data)
      } catch {
        if (!cancelled) toast.error(t('connections.notFound'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [isCreate, connectionID, institution, direction, t])

  // 编辑态（第二步）：类型就绪后回填表单（敏感字段留空 = 不修改）
  useEffect(() => {
    if (isCreate || !record || !typeInfo) return
    {
        const next: Record<string, FieldValue> = {}
        // 出方向的类型专属配置存在 connector.config 里（通用字段仍在实体上）：
        // 敏感键不回填明文（后端只下发 has_secrets），编辑态留空即"不修改"。
        const connectorConfig = (record as unknown as { config?: Record<string, string> }).config ?? {}
        const genericFields = new Set(['name', 'display_name', 'application', 'state', 'just_in_time'])
        for (const field of [...basicFields, ...configFields, ...scopeFields]) {
          if (direction === 'out' && !genericFields.has(field.key)) {
            if (field.secret) continue
            const configured = connectorConfig[field.key]
            if (configured === undefined || configured === '') continue
            next[field.key] = configured
            continue
          }
          const raw = readStored((record as unknown as Record<string, unknown>), field.store || field.key)
          if (raw === undefined || raw === null) continue
          if (field.type === 'switch') next[field.key] = raw === true
          else if (field.type === 'tags') next[field.key] = Array.isArray(raw) ? raw.map(String) : []
          else next[field.key] = String(raw)
        }
        next['is_enabled'] = (record as unknown as Record<string, unknown>)['is_enabled'] === true
        setValues(next)
        const rawMapping = (record as unknown as Record<string, unknown>)['user_mapping']
        if (rawMapping && typeof rawMapping === 'object') {
          setMapping(rawMapping as Record<string, string>)
        }
    }
  }, [isCreate, record, typeInfo, direction, basicFields, configFields, scopeFields])

  // 步骤：仅在类型声明支持字段映射时才出现「字段映射」一步（不支持的连接器不展示空表格）
  const stepIds = useMemo(() => {
    const ids = ['basic', 'config', 'scope']
    if (typeInfo?.capabilities.mapping) ids.push('mapping')
    return ids
  }, [typeInfo])
  const steps: WizardStep[] = useMemo(
    () => stepIds.map((id) => STEP_DEFS[id]),
    [stepIds],
  )

  const setField = (key: string, value: FieldValue) => setValues((prev) => ({ ...prev, [key]: value }))

  const pickType = (item: ConnectionCatalogItem) => {
    setTypeKey(item.typeKey)
    setSubType(item.subType ?? '')
    setStep(0)
  }

  const validateBasic = (): string | null => {
    const name = String(values['name'] ?? '')
    if (!name) return 'connections.errNameRequired'
    if (isCreate && !NAME_PATTERN.test(name)) return 'connections.errNamePattern'
    return null
  }

  const validateSchedule = (): string | null => {
    const mode = String(values['schedule_mode'] ?? '')
    if (mode === 'interval') {
      const seconds = Number(values['schedule_interval_seconds'] ?? 0)
      if (!seconds || seconds <= 0) return 'connections.errScheduleInterval'
    }
    if (mode === 'cron') {
      const expression = String(values['schedule_cron'] ?? '').trim()
      if (expression.split(/\s+/).filter(Boolean).length !== 5) return 'connections.errScheduleCron'
    }
    if (mode === 'daily') {
      const times = Array.isArray(values['schedule_times']) ? values['schedule_times'] : []
      if (times.length === 0) return 'connections.errScheduleTimes'
      if (times.length > 24) return 'connections.errScheduleTooMany'
      for (const item of times) {
        if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(item)) return 'connections.errScheduleFormat'
      }
    }
    return null
  }

  const validateConfig = (): string | null => {
    for (const field of configFields) {
      if (!field.required) continue
      if (String(values[field.key] ?? '').trim() === '' && !(field.secret && !isCreate)) {
        return t('connections.errFieldRequired', {
          field: i18nLabel(field, t),
        })
      }
    }
    return null
  }

  const save = async () => {
    if (!typeInfo) return
    const basicError = validateBasic()
    if (basicError) {
      toast.error(t(basicError))
      setStep(0)
      return
    }
    const scheduleError = validateSchedule()
    if (scheduleError) {
      toast.error(t(scheduleError))
      setStep(2)
      return
    }
    const configError = validateConfig()
    if (configError) {
      toast.error(configError)
      setStep(1)
      return
    }
    const ok = isCreate
      ? await run(() => createConnection(typeInfo, values, mapping, direction, institution), 'connections.created')
      : await run(() => updateConnection(typeInfo, values, mapping, direction, connectionID), 'common.saved')
    if (ok) navigate(backToList)
  }

  if (catalog && !typeInfo && isCreate) {
    // 创建时未带类型：**整页**选择接入方式（不使用弹窗）
    // 注意：编辑态类型来自记录，不能落到这里（否则"编辑"会变成"重新选类型"）
    const items = toCatalogItems(
      catalog.directions.find((group) => group.direction === direction)?.types ?? [],
    )
    return (
      <PageShell titleKey="connections.addTitle" subtitleKey="connections.catalogHint">
        <div className="space-y-4">
          <Button variant="ghost" size="sm" onClick={() => navigate(backToList)}>
            <ArrowLeft className="size-4" /> {t('common.back')}
          </Button>
          <ConnectionCatalogGrid items={items} onPick={pickType} />
        </div>
      </PageShell>
    )
  }

  if (!isCreate && !loading && !record) {
    return (
      <PageShell titleKey="connections.editTitle" subtitleKey="connections.subtitle">
        <div className="bg-card border-border/70 shadow-soft space-y-3 rounded-2xl border p-8 text-center">
          <p className="text-sm font-medium">{t('connections.notFound')}</p>
          <Button variant="outline" onClick={() => navigate(backToList)}>
            <ArrowLeft className="size-4" /> {t('common.back')}
          </Button>
        </div>
      </PageShell>
    )
  }

  return (
    <PageShell
      titleKey={isCreate ? 'connections.createTitle' : 'connections.editTitle'}
      subtitleKey="connections.subtitle"
    >
      {loading ? (
        <Skeleton className="h-64 w-full" />
      ) : (
        <FormWizard
          steps={steps}
          current={step}
          onStepChange={setStep}
          onFinish={save}
          finishLabelKey={isCreate ? 'connections.create' : 'common.save'}
          onCancel={() => navigate(backToList)}
          pending={pending}
        >
          {/* 字段映射是宽表（每行"字段名 + 规则 + 取值 + 状态 + 操作"），内容区在它这步放宽 */}
          <div className={(stepIds[step] === 'mapping' ? 'max-w-5xl' : 'max-w-2xl') + ' space-y-6'}>
            {/*
              接入方（类型）只在第一步出现一次：类型已在目录页选定，不必每一步都重复同一张卡片。
              创建态保留「更换接入方式」，编辑态只作标识展示。
            */}
            {stepIds[step] === 'basic' && typeInfo ? (
              <div className="bg-card border-border/70 shadow-soft flex items-center gap-3 rounded-2xl border p-4">
                <ConnectionTile icon={typeInfo.icon} type={typeInfo.key} subType={subType} tileSize={40} />
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold">{typeInfo.name}</p>
                  <p className="text-muted-foreground truncate text-[11px]">
                    {typeInfo.key}
                    {subType ? ` · ${subType}` : ''}
                  </p>
                </div>
                {isCreate ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    className="ml-auto"
                    onClick={() => {
                      setTypeKey('')
                      setSubType('')
                    }}
                  >
                    {t('connections.changeType')}
                  </Button>
                ) : null}
              </div>
            ) : null}

            {stepIds[step] === 'basic' ? (
              <ConnectionFields typeKey={typeInfo?.key} fields={basicFields} values={values} onChange={setField} editing={!isCreate} />
            ) : null}
            {stepIds[step] === 'config' ? (
              <ConnectionFields typeKey={typeInfo?.key} fields={configFields} values={values} onChange={setField} editing={!isCreate} />
            ) : null}
            {stepIds[step] === 'mapping' ? (
              <ConnectionMapping
                targetFields={catalog?.target_fields ?? []}
                sourceFields={typeInfo?.source_fields ?? []}
                value={mapping}
                onChange={setMapping}
              />
            ) : null}
            {stepIds[step] === 'scope' ? (
              <>
                <ConnectionFields
                  fields={scopeFields}
                  values={values}
                  onChange={setField}
                  editing={!isCreate}
                  departments={departments}
                />
                {direction === 'in' ? (
                  <p className="text-muted-foreground text-xs">{t('connections.scopeHint')}</p>
                ) : null}
              </>
            ) : null}
          </div>
        </FormWizard>
      )}
    </PageShell>
  )
}

/** 向导步骤定义（步骤顺序由能力动态决定） */
const STEP_DEFS: Record<string, WizardStep> = {
  basic: { key: 'basic', titleKey: 'connections.stepBasic', descriptionKey: 'connections.stepBasicDesc' },
  config: { key: 'config', titleKey: 'connections.stepConfig', descriptionKey: 'connections.stepConfigDesc' },
  scope: { key: 'scope', titleKey: 'connections.stepScope', descriptionKey: 'connections.stepScopeDesc' },
  mapping: { key: 'mapping', titleKey: 'connections.stepMapping', descriptionKey: 'connections.stepMappingDesc' },
}

/** 字段标签（与 ConnectionFields 同源：优先 i18n 词条，回退后端标签） */
function i18nLabel(field: ConnectionField, t: (key: string) => string): string {
  const key = `connections.field.${field.key}`
  return i18n.exists(key) ? t(key) : field.label
}

/** 把向导表单值转成入方向创建/更新载荷 */
/**
 * 把向导表单值转成入方向载荷。
 * 落库路径取自后端目录的 `store`（如 `sync_config.schedule.mode`），因此"界面上填的字段"
 * 与"后端读的字段"由同一份声明决定，不存在两处对不上的可能。
 */
function toSyncerPayload(
  type: ConnectionTypeInfo,
  values: FieldValueMap,
  mapping: Record<string, string>,
  institution: string,
): SyncerPayload {
  const payload: SyncerPayload = {
    institution,
    name: String(values['name'] ?? ''),
    display_name: String(values['display_name'] ?? ''),
    type: type.key,
    is_enabled: values['is_enabled'] === true,
    schedule: '', // 定时统一由 sync_config.schedule 表达（历史字段仅兼容读取）
  }
  const bag = payload as unknown as Record<string, unknown>
  const assign = (path: string, value: unknown) => {
    const parts = path.split('.')
    let cursor = bag
    for (const part of parts.slice(0, -1)) {
      if (typeof cursor[part] !== 'object' || cursor[part] === null) cursor[part] = {}
      cursor = cursor[part] as Record<string, unknown>
    }
    cursor[parts[parts.length - 1]] = value
  }

  for (const field of type.fields) {
    const value = values[field.key]
    if (field.secret) {
      const plain = typeof value === 'string' ? value : ''
      if (!plain) continue // 留空表示不修改
      if (field.key === 'app_secret') payload.app_secret_plain = plain
      else payload.secret = plain
      continue
    }
    const path = field.store || field.key
    switch (field.type) {
      case 'switch':
        assign(path, value === true)
        break
      case 'number': {
        const num = Number(value ?? 0)
        if (!Number.isFinite(num)) continue
        assign(path, num)
        break
      }
      case 'tags': {
        const items = (Array.isArray(value) ? value : []).map((item) => String(item).trim()).filter(Boolean)
        if (!items.length) continue
        assign(path, items)
        break
      }
      case 'department':
        // 组织选择：空 = 组织根（显式下发 null，便于"清空原有设置"）
        assign(path, value ? Number(value) : null)
        break
      default: {
        const text = typeof value === 'string' ? value : ''
        if (text === '') continue
        assign(path, text)
      }
    }
  }
  if (type.capabilities.mapping && Object.keys(mapping).length > 0) {
    payload.user_mapping = mapping
  }
  return payload
}

/** 把向导表单值转成出方向创建/更新载荷 */
/** 按点分路径读取已存值（回填编辑态用） */
function readStored(source: Record<string, unknown>, path: string): unknown {
  return path.split('.').reduce<unknown>((node, part) => {
    if (typeof node !== 'object' || node === null) return undefined
    return (node as Record<string, unknown>)[part]
  }, source)
}

function toConnectorPayload(
  type: ConnectionTypeInfo,
  values: FieldValueMap,
  _mapping: Record<string, string>,
  institution: string,
): ConnectorPayload {
  const payload: ConnectorPayload = {
    institution,
    name: String(values['name'] ?? ''),
    display_name: String(values['display_name'] ?? ''),
    application: String(values['application'] ?? ''),
    type: type.key,
    state: String(values['state'] ?? 'active'),
    config: {},
    secrets: {},
  }
  // 通用字段由连接器实体承载；其余按目录声明分流到 config（非敏感）与 secrets（敏感）
  const generic = new Set(['name', 'display_name', 'application', 'state', 'just_in_time'])
  for (const field of type.fields) {
    if (generic.has(field.key)) continue
    const value = values[field.key]
    if (field.secret) {
      const plain = typeof value === 'string' ? value.trim() : ''
      if (plain) payload.secrets![field.key] = plain // 留空表示不修改
      continue
    }
    const text = Array.isArray(value) ? value.join(',') : typeof value === 'string' ? value.trim() : ''
    if (text) payload.config![field.key] = text
  }
  return payload
}

async function createConnection(
  type: ConnectionTypeInfo,
  values: FieldValueMap,
  mapping: Record<string, string>,
  direction: ConnectionDirection,
  institution: string,
): Promise<unknown> {
  return direction === 'in'
    ? syncerApi.create(toSyncerPayload(type, values, mapping, institution))
    : connectorApi.create(toConnectorPayload(type, values, mapping, institution))
}

async function updateConnection(
  type: ConnectionTypeInfo,
  values: FieldValueMap,
  mapping: Record<string, string>,
  direction: ConnectionDirection,
  id: number,
): Promise<unknown> {
  return direction === 'in'
    ? syncerApi.update(id, toSyncerPayload(type, values, mapping, ''))
    : connectorApi.update(id, toConnectorPayload(type, values, mapping, ''))
}
