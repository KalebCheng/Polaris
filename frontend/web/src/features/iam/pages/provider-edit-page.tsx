import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { describeError } from '@/shared/lib/errors'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { toast } from '@/shared/ui/toast'
import { ArrowLeft, Plug } from 'lucide-react'
import {
  Button,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
  Switch,
  Textarea,
} from '@/shared/ui'
import { TagInput } from '@/shared/components/tag-input'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { PageShell } from '@/widgets/layout/page-shell'
import { FormWizard, type WizardStep } from '@/shared/components/form-wizard'
import { SchemaFields, type FieldValueMap } from '@/shared/components/schema-fields'
import { CopyButton } from '@/shared/components/copy-button'
import { ProviderIcon, ProviderTile } from '@/shared/components/provider-icon'
import i18n from '@/shared/i18n'
import type {
  ConnectionCatalog,
  Provider,
  ProviderCatalog,
  ProviderField,
  ProviderTemplate,
  ProviderTypeInfo,
  Syncer,
} from '@/shared/api/types'
import { connectionApi, platformApi, providerApi, syncerApi, type ProviderPayload } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { useMutation } from '../hooks/use-mutation'
import {
  ProviderCatalogGrid,
  groupByCategory,
  toCatalogItems,
  type ProviderCatalogItem,
} from '../components/provider-catalog'

type FieldValue = string | boolean | string[]

const NAME_PATTERN = /^[a-z0-9][a-z0-9-_]{1,30}$/

/** 步骤定义：字段按后端目录的 group 分派（basic / connection / advanced） */
const STEP_DEFS: Record<string, WizardStep> = {
  basic: { key: 'basic', titleKey: 'providers.stepBasic', descriptionKey: 'providers.stepBasicHint' },
  connection: {
    key: 'connection',
    titleKey: 'providers.stepConnection',
    descriptionKey: 'providers.stepConnectionHint',
  },
  advanced: {
    key: 'advanced',
    titleKey: 'providers.stepAdvanced',
    descriptionKey: 'providers.stepAdvancedHint',
  },
}

/**
 * 提供商 新建 / 编辑页（**整页向导**，与「身份连接」一致，不使用弹窗）。
 * 路由：/institutions/:inst/providers/new?category=&type= 与 /institutions/:inst/providers/:id
 *
 * 字段全部来自后端目录（`GET /providers/catalog`）：目录的 `group` 决定字段出现在哪一步，
 * `secret` 决定它进 secrets 桶（留空 = 不修改），`policy` 决定它进 SSO 信任策略。
 * 因此"界面上填的字段"与"后端读的字段"由同一份声明决定。
 */
export default function ProviderEditPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()
  const navigate = useNavigate()
  const { id = '' } = useParams()
  const [params] = useSearchParams()
  const isCreate = !id || id === 'new'
  const providerID = Number(id)

  const [categoryKey, setCategoryKey] = useState(params.get('category') ?? '')
  const [typeKey, setTypeKey] = useState(params.get('type') ?? '')
  const [catalog, setCatalog] = useState<ProviderCatalog | null>(null)
  const [step, setStep] = useState(0)
  const [values, setValues] = useState<FieldValueMap>({})
  const [loading, setLoading] = useState(!isCreate)
  // record 编辑态加载到的实例（类型由它定位，见上方两个 effect）
  const [record, setRecord] = useState<Provider | null>(null)
  const [testing, setTesting] = useState(false)
  const [smsProviders, setSmsProviders] = useState<Provider[]>([])
  const [smsTemplates, setSmsTemplates] = useState<ProviderTemplate[]>([])
  const [syncers, setSyncers] = useState<Syncer[]>([])
  const [connectionCatalog, setConnectionCatalog] = useState<ConnectionCatalog | null>(null)
  const { pending, run } = useMutation()

  const backToList = `/institutions/${institution}/providers`

  // 目录：类别 / 类型 / 字段 schema 的唯一来源
  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const data = await providerApi.catalog()
        if (!cancelled) setCatalog(data)
      } catch {
        if (!cancelled) setCatalog(null)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const typeInfo: ProviderTypeInfo | undefined = useMemo(() => {
    const category = catalog?.categories.find((item) => item.key === categoryKey)
    return category?.types.find((item) => item.key === typeKey)
  }, [catalog, categoryKey, typeKey])

  // 引用必须稳定：typeInfo 未就绪时若每次渲染都新建数组，下方 applyDefaults 会与渲染互相触发
  const basicFields = useMemo(() => (typeInfo?.fields ?? []).filter((f) => f.group === 'basic'), [typeInfo])
  const connectionFields = useMemo(
    () => (typeInfo?.fields ?? []).filter((f) => f.group === 'connection'),
    [typeInfo],
  )
  const advancedFields = useMemo(
    () => (typeInfo?.fields ?? []).filter((f) => f.group === 'advanced'),
    [typeInfo],
  )

  const applyDefaults = useCallback((fields: ProviderField[]) => {
    setValues((prev) => {
      const next = { ...prev }
      for (const field of fields) {
        if (next[field.key] !== undefined) continue
        if (field.type === 'switch') next[field.key] = field.default === 'true'
        else if (field.type === 'tags' || field.type === 'multiselect') next[field.key] = []
        else next[field.key] = field.default ?? ''
      }
      return next
    })
  }, [])

  useEffect(() => {
    applyDefaults([...basicFields, ...connectionFields, ...advancedFields])
  }, [applyDefaults, basicFields, connectionFields, advancedFields])

  // 编辑态（第一步）：按 id 取记录，并用记录里的 category/type 定位类型。
  // **编辑地址只带 id**（不像新建那样在 URL 里带 category/type）；若先等 typeInfo 再取记录会死锁，
  // 于是"编辑"会一直停在类型选择页——这正是本次修复的缺陷。
  useEffect(() => {
    if (isCreate || providerID <= 0 || !institution) return
    let cancelled = false
    setLoading(true)
    void (async () => {
      try {
        const data = await providerApi.get(providerID, institution)
        if (cancelled) return
        setCategoryKey(data.category)
        setTypeKey(data.type)
        setRecord(data)
      } catch {
        if (!cancelled) toast.error(t('providers.notFound'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [isCreate, providerID, institution, t])

  // 编辑态（第二步）：类型就绪后回填表单（密钥不回填，留空即"不修改"）
  useEffect(() => {
    if (isCreate || !record || !typeInfo) return
    const next: Record<string, FieldValue> = {}
    for (const field of [...basicFields, ...connectionFields, ...advancedFields]) {
      if (field.secret) continue
      const bucket = field.policy ? record.policy : record.config
      const raw = bucket?.[field.key]
      if (raw === undefined || raw === null) continue
      if (field.type === 'switch') next[field.key] = raw === true
      else if (field.type === 'tags' || field.type === 'multiselect')
        next[field.key] = Array.isArray(raw) ? raw.map(String) : []
      else next[field.key] = String(raw)
    }
    next['name'] = record.name
    next['display_name'] = record.display_name
    next['state'] = record.state
    setValues(next)
  }, [isCreate, record, typeInfo, basicFields, connectionFields, advancedFields])

  // 步骤：无高级项的类型不展示「高级与策略」一步
  const stepIds = useMemo(() => {
    const ids = ['basic', 'connection']
    if (advancedFields.length > 0) ids.push('advanced')
    return ids
  }, [advancedFields])
  const steps: WizardStep[] = useMemo(() => stepIds.map((key) => STEP_DEFS[key]), [stepIds])

  const setField = (key: string, value: FieldValue) =>
    setValues((prev) => {
      const next = { ...prev, [key]: value }
      if (key === 'match_type') {
        if (value === 'system') {
          next['match_syncer_id'] = ''
          next['match_syncer_field'] = ''
        } else if (value === 'identity_link') {
          next['match_local_field'] = ''
        } else {
          next['match_local_field'] = ''
          next['match_syncer_id'] = ''
          next['match_syncer_field'] = ''
          next['match_provider_field'] = ''
        }
      }
      return next
    })

  const pickType = (item: ProviderCatalogItem) => {
    setCategoryKey(item.categoryKey)
    setTypeKey(item.typeKey)
    setStep(0)
  }

  const validateBasic = (): string | null => {
    const name = String(values['name'] ?? '')
    if (!name) return 'providers.errNameRequired'
    if (isCreate && !NAME_PATTERN.test(name)) return 'providers.errNamePattern'
    return null
  }

  const validateConnection = (): string | null => {
    for (const field of connectionFields) {
      if (!field.required) continue
      const raw = String(values[field.key] ?? '').trim()
      if (raw === '' && !(field.secret && !isCreate)) {
        return t('providers.errFieldRequired', { field: labelOf(field, t) })
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
    const connectionError = validateConnection()
    if (connectionError) {
      toast.error(connectionError)
      setStep(1)
      return
    }
    const payload = toPayload(typeInfo, values)
    const ok = isCreate
      ? await run(() => providerApi.create(payload), 'providers.created')
      : await run(() => providerApi.update(providerID, payload), 'common.saved')
    if (ok) navigate(backToList)
  }

  const checkNow = async () => {
    if (!providerID) return
    setTesting(true)
    await run(() => providerApi.test(providerID), 'providers.testOk')
    setTesting(false)
  }

  useEffect(() => {
    if (typeKey !== 'sms' || categoryKey !== 'mfa') return
    let cancelled = false
    void providerApi
      .list({ category: 'sms', size: 200 })
      .then((data) => {
        if (!cancelled) setSmsProviders(data.items ?? [])
      })
      .catch(() => {
        if (!cancelled) setSmsProviders([])
      })
    return () => {
      cancelled = true
    }
  }, [typeKey, categoryKey])

  useEffect(() => {
    if (categoryKey !== 'oauth') return
    let cancelled = false
    void syncerApi
      .list({ page: 1, size: 200 })
      .then((data) => {
        if (!cancelled) setSyncers(data.items ?? [])
      })
      .catch(() => {
        if (!cancelled) setSyncers([])
      })
    return () => {
      cancelled = true
    }
  }, [categoryKey, institution])

  // 「身份连接字段」的候选来自所选入方向连接的源字段目录（不在前端硬编码）。
  useEffect(() => {
    if (categoryKey !== 'oauth') return
    let cancelled = false
    void connectionApi
      .catalog()
      .then((data) => {
        if (!cancelled) setConnectionCatalog(data)
      })
      .catch(() => {
        if (!cancelled) setConnectionCatalog(null)
      })
    return () => {
      cancelled = true
    }
  }, [categoryKey])

  useEffect(() => {
    const smsProviderID = Number(values['mfa_sms_provider_id'] ?? 0)
    if (typeKey !== 'sms' || categoryKey !== 'mfa' || !smsProviderID) {
      setSmsTemplates([])
      return
    }
    let cancelled = false
    void providerApi
      .templates(smsProviderID, 'mfa_code')
      .then((data) => {
        if (!cancelled) setSmsTemplates(data.items ?? [])
      })
      .catch(() => {
        if (!cancelled) setSmsTemplates([])
      })
    return () => {
      cancelled = true
    }
  }, [typeKey, categoryKey, values])

  // 所选身份连接的源字段 → 「身份连接字段」下拉；external_id 是所有连接都有的映射主键，始终可选。
  const matchSyncerFieldOptions = useMemo(() => {
    const selected = syncers.find((item) => String(item.id) === String(values['match_syncer_id'] ?? ''))
    const inbound = connectionCatalog?.directions
      .find((group) => group.direction === 'in')
      ?.types.find((type) => type.key === selected?.type)
    const sourceFields = (inbound?.source_fields ?? []).map((field) => ({
      value: field.key,
      label: `${field.label}（${field.key}）`,
    }))
    return [{ value: 'external_id', label: '外部唯一标识（external_id）' }, ...sourceFields]
  }, [syncers, values, connectionCatalog])

  const dynamicOptions = useMemo(
    () => ({
      mfa_sms_provider_id: smsProviders.map((item) => ({
        value: String(item.id),
        label: item.display_name || item.name,
      })),
      mfa_sms_template_id: smsTemplates.map((item) => ({ value: String(item.id), label: item.name })),
      match_syncer_id: syncers.map((item) => ({
        value: String(item.id),
        label: item.display_name || item.name,
      })),
      match_syncer_field: matchSyncerFieldOptions,
    }),
    [smsProviders, smsTemplates, syncers, matchSyncerFieldOptions],
  )

  if (catalog && !typeInfo && isCreate) {
    // 未选定类型：**整页**目录（不使用弹窗）
    // 注意：编辑态的类型来自记录，不能落到这里（否则"编辑"会变成"重新选类型"）
    const items = toCatalogItems(catalog.categories)
    const groups = groupByCategory(catalog.categories, items)
    return (
      <PageShell titleKey="providers.addTitle" subtitleKey="providers.catalogHint">
        <div className="space-y-6">
          <Button variant="ghost" size="sm" onClick={() => navigate(backToList)}>
            <ArrowLeft className="size-4" /> {t('common.back')}
          </Button>
          {groups.map((group) => (
            <div key={group.category.key} className="space-y-3">
              <div className="space-y-0.5">
                <h2 className="flex items-center gap-1.5 text-sm font-semibold">
                  <ProviderIcon iconKey={group.category.icon || group.category.key} size={16} />
                  {t(`providers.category.${group.category.key}`)}
                </h2>
                <p className="text-muted-foreground text-xs">
                  {t(`providers.categoryHint.${group.category.key}`)}
                </p>
              </div>
              <ProviderCatalogGrid items={group.items} onPick={pickType} />
            </div>
          ))}
        </div>
      </PageShell>
    )
  }

  if (!isCreate && !loading && !record) {
    return (
      <PageShell titleKey="providers.editTitle" subtitleKey="providers.subtitle">
        <div className="surface space-y-3 rounded-2xl p-8 text-center">
          <p className="text-sm font-medium">{t('providers.notFound')}</p>
          <Button variant="outline" onClick={() => navigate(backToList)}>
            <ArrowLeft className="size-4" /> {t('common.back')}
          </Button>
        </div>
      </PageShell>
    )
  }

  return (
    <PageShell
      titleKey={isCreate ? 'providers.createTitle' : 'providers.editTitle'}
      subtitleKey="providers.subtitle"
    >
      {loading ? (
        <Skeleton className="h-64 w-full" />
      ) : (
        <FormWizard
          steps={steps}
          current={step}
          onStepChange={setStep}
          onFinish={save}
          finishLabelKey={isCreate ? 'providers.create' : 'common.save'}
          onCancel={() => navigate(backToList)}
          pending={pending}
        >
          <div className="max-w-2xl space-y-6">
            {/*
              类型（接入方）只在第一步出现一次：类型已在目录页选定，不必每一步都重复同一张卡片。
              创建态保留「更换类型」；编辑态只作标识展示。
              （AGENTS §8 教训：步骤内容必须写在步骤条件内，否则每步都会重复渲染）
            */}
            {stepIds[step] === 'basic' && typeInfo ? (
              <div className="surface flex items-center gap-3 rounded-2xl p-4">
                <ProviderTile iconKey={typeInfo.icon} tileSize={40} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-semibold">{typeInfo.name}</p>
                  <p className="text-muted-foreground truncate text-xs">
                    {t(`providers.category.${typeInfo.category}`)} · {typeInfo.key}
                  </p>
                </div>
                {isCreate ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setCategoryKey('')
                      setTypeKey('')
                      setStep(0)
                    }}
                  >
                    {t('providers.changeType')}
                  </Button>
                ) : null}
              </div>
            ) : null}

            {stepIds[step] === 'basic' ? (
              <SchemaFields
                fields={basicFields}
                values={values}
                onChange={setField}
                i18nPrefix="providers"
                typeKey={typeInfo?.key}
                editing={!isCreate}
                dynamicOptions={dynamicOptions}
              />
            ) : null}

            {stepIds[step] === 'connection' ? (
              <SchemaFields
                fields={connectionFields}
                values={values}
                onChange={setField}
                i18nPrefix="providers"
                typeKey={typeInfo?.key}
                editing={!isCreate}
                dynamicOptions={dynamicOptions}
              />
            ) : null}

            {stepIds[step] === 'advanced' ? (
              <div className="space-y-6">
                {typeInfo?.capabilities.signin ? (
                  <SigninDevInfo override={String(values['redirect_uri'] ?? '')} />
                ) : null}
                <SchemaFields
                  fields={advancedFields}
                  values={values}
                  onChange={setField}
                  i18nPrefix="providers"
                  typeKey={typeInfo?.key}
                  editing={!isCreate}
                  dynamicOptions={dynamicOptions}
                />
                {providerID > 0 && (categoryKey === 'email' || categoryKey === 'sms') ? (
                  <ProviderTemplatesSection providerID={providerID} category={categoryKey} />
                ) : null}
                {providerID > 0 ? (
                  <div className="flex items-center gap-2">
                    <Button type="button" variant="outline" size="sm" disabled={testing} onClick={checkNow}>
                      <Plug className="size-4" /> {testing ? t('providers.testing') : t('providers.test')}
                    </Button>
                    <p className="text-muted-foreground text-xs">{t('providers.testHint')}</p>
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
        </FormWizard>
      )}
    </PageShell>
  )
}

function ProviderTemplatesSection({ providerID, category }: { providerID: number; category: string }) {
  const { t } = useTranslation()
  const [items, setItems] = useState<ProviderTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [savingID, setSavingID] = useState<number | null>(null)
  const [deleting, setDeleting] = useState<ProviderTemplate | null>(null)

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const data = await providerApi.templates(providerID)
      setItems(data.items ?? [])
    } finally {
      setLoading(false)
    }
  }, [providerID])

  useEffect(() => {
    void reload()
  }, [reload])

  const patch = (id: number, next: Partial<ProviderTemplate>) =>
    setItems((prev) => prev.map((item) => (item.id === id ? { ...item, ...next } : item)))

  const add = () =>
    setItems((prev) => [
      {
        id: 0,
        institution_id: 0,
        provider_id: providerID,
        name: '',
        scene: 'login_code',
        template_code: '',
        template_params: [],
        subject: '',
        content: '',
        is_default: false,
        created_at: '',
        updated_at: '',
      },
      ...prev,
    ])

  const save = async (item: ProviderTemplate) => {
    setSavingID(item.id)
    try {
      const body = {
        name: item.name,
        scene: item.scene,
        template_code: item.template_code,
        template_params: item.template_params,
        subject: item.subject,
        content: item.content,
        is_default: item.is_default,
      }
      if (item.id > 0) await providerApi.updateTemplate(providerID, item.id, body)
      else await providerApi.createTemplate(providerID, body)
      toast.success(t('common.saved'))
      await reload()
    } catch (error) {
      toast.error(describeError(error, t))
    } finally {
      setSavingID(null)
    }
  }

  const confirmDelete = async () => {
    if (!deleting) return
    try {
      await providerApi.removeTemplate(providerID, deleting.id)
      toast.success(t('providers.templates.deleted'))
      await reload()
    } catch (error) {
      toast.error(describeError(error, t))
    } finally {
      setDeleting(null)
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-medium">{t('providers.templates.title')}</p>
          <p className="text-muted-foreground text-xs">{t('providers.templates.hint')}</p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={add}>
          {t('providers.templates.add')}
        </Button>
      </div>
      {loading ? <Skeleton className="h-24 w-full" /> : null}
      {!loading && items.length === 0 ? (
        <p className="text-muted-foreground text-xs">{t('providers.templates.empty')}</p>
      ) : null}
      {items.map((item) => (
        <div
          key={item.id || `new-${item.name}`}
          className="bg-muted/30 border-border/70 space-y-3 rounded-xl border p-3"
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <Input
              value={item.name}
              placeholder={t('providers.templates.name')}
              onChange={(event) => patch(item.id, { name: event.target.value })}
            />
            <Select
              value={item.scene}
              onValueChange={(value) => patch(item.id, { scene: value as ProviderTemplate['scene'] })}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="login_code">login_code</SelectItem>
                {category === 'sms' ? <SelectItem value="mfa_code">mfa_code</SelectItem> : null}
                {category === 'email' ? <SelectItem value="magic_link">magic_link</SelectItem> : null}
              </SelectContent>
            </Select>
          </div>
          {category === 'sms' ? (
            <>
              <Input
                value={item.template_code}
                placeholder={t('providers.templates.code')}
                onChange={(event) => patch(item.id, { template_code: event.target.value })}
              />
              <TagInput
                value={item.template_params}
                onChange={(next) => patch(item.id, { template_params: next })}
                placeholder={t('providers.templates.params')}
              />
            </>
          ) : (
            <>
              <Input
                value={item.subject}
                placeholder={t('providers.templates.subject')}
                onChange={(event) => patch(item.id, { subject: event.target.value })}
              />
              <Textarea
                value={item.content}
                rows={4}
                placeholder={t('providers.templates.content')}
                onChange={(event) => patch(item.id, { content: event.target.value })}
              />
            </>
          )}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <label className="flex items-center gap-2 text-xs">
              <Switch
                checked={item.is_default}
                onCheckedChange={(checked) => patch(item.id, { is_default: checked })}
              />
              {t('providers.templates.isDefault')}
            </label>
            <div className="flex items-center gap-2">
              <Button type="button" size="sm" disabled={savingID === item.id} onClick={() => void save(item)}>
                {t('common.save')}
              </Button>
              {item.id > 0 ? (
                <Button type="button" variant="outline" size="sm" onClick={() => setDeleting(item)}>
                  {t('common.delete')}
                </Button>
              ) : null}
            </div>
          </div>
        </div>
      ))}
      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(open) => !open && setDeleting(null)}
        titleKey="providers.templates.deleteTitle"
        descriptionKey="providers.templates.deleteHint"
        onConfirm={() => void confirmDelete()}
      />
    </div>
  )
}

/** 字段标签：优先 i18n，缺词条时回退目录中文标签 */
function labelOf(field: ProviderField, t: (key: string) => string): string {
  const key = `providers.field.${field.key}`
  return i18n.exists(key) ? t(key) : field.label
}

/**
 * 开发信息：把平台的回调地址与门户地址原样填到 IdP 后台。
 * 取值来自后端（与登录链路同一个 resolveRedirectURI 实现），因此界面上写的就是实际用的地址。
 */
function SigninDevInfo({ override }: { override?: string }) {
  const { t } = useTranslation()
  const [info, setInfo] = useState<{ callback_url: string; homepage_url: string } | null>(null)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const data = await platformApi.oauthInfo()
        if (!cancelled) setInfo(data)
      } catch {
        if (!cancelled) setInfo(null)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  if (!info) return null
  // 表单里填了「回调地址覆盖」时，开发信息要立即跟着变；否则管理员会以为覆盖没生效。
  const callbackURL = resolveCallbackURL(info.callback_url, override ?? '')
  const rows = [
    { key: 'providers.devCallback', value: callbackURL },
    { key: 'providers.devHomepage', value: info.homepage_url },
  ]
  // 部署在公网、但后端仍返回 loopback 地址：明确提示配置项，避免管理员把 127.0.0.1 填进 IdP 后台。
  const callbackIsLoopback = isLoopbackURL(callbackURL)
  const pageIsLoopback = isLoopbackHost(window.location.hostname)
  const showLoopbackWarning = callbackIsLoopback && !pageIsLoopback
  return (
    <div className="bg-muted/40 border-border/70 space-y-3 rounded-xl border p-3">
      <div className="space-y-0.5">
        <p className="text-[13px] font-medium">{t('providers.devInfo')}</p>
        <p className="text-muted-foreground text-[11px]">{t('providers.devInfoHint')}</p>
      </div>
      {rows.map((row) => (
        <div key={row.key} className="space-y-1">
          <p className="text-[12px] font-medium">{t(row.key)}</p>
          <div className="bg-background flex items-center gap-2 rounded-md border px-2 py-1.5">
            <code className="min-w-0 flex-1 truncate font-mono text-[11px]">{row.value}</code>
            <CopyButton value={row.value} label={t(row.key)} />
          </div>
        </div>
      ))}
      {showLoopbackWarning ? (
        <p className="text-destructive text-[11px] leading-relaxed">
          {t('providers.devInfoLoopbackWarning')}
        </p>
      ) : null}
    </div>
  )
}

function resolveCallbackURL(fallback: string, override: string): string {
  const raw = override.trim()
  if (!raw) return fallback
  if (raw.startsWith('/')) {
    try {
      return new URL(raw, fallback).toString()
    } catch {
      return raw
    }
  }
  return raw
}

function isLoopbackHost(host: string): boolean {
  return ['localhost', '127.0.0.1', '::1', '[::1]'].includes(host.toLowerCase())
}

function isLoopbackURL(raw: string): boolean {
  try {
    return isLoopbackHost(new URL(raw).hostname)
  } catch {
    return false
  }
}

/**
 * 把向导值按目录声明分桶：secret → secrets，policy → policy，其余 → config。
 * 落库列由后端目录的 store 决定，前端只负责"把值放进正确的桶"。
 */
/** 顶层字段：走请求体的一级键，不进入 config 桶 */
const TOP_LEVEL_FIELDS = new Set(['name', 'display_name', 'state'])

export function toPayload(type: ProviderTypeInfo, values: FieldValueMap): ProviderPayload {
  const payload: ProviderPayload = {
    name: String(values['name'] ?? '').trim(),
    display_name: String(values['display_name'] ?? '').trim(),
    category: type.category,
    type: type.key,
    state: String(values['state'] ?? 'active') || 'active',
    config: {},
    secrets: {},
    policy: {},
  }
  for (const field of type.fields) {
    // 标识 / 名称 / 状态由顶层字段承载，不再重复进 config（避免同一列被两个桶写入）
    if (TOP_LEVEL_FIELDS.has(field.key)) continue
    const value = values[field.key]
    if (field.secret) {
      const plain = typeof value === 'string' ? value.trim() : ''
      if (plain) payload.secrets![field.key] = plain
      continue
    }
    if (field.type === 'switch') {
      if (value === undefined) continue
      assign(payload, field, value === true)
      continue
    }
    if (field.type === 'tags' || field.type === 'multiselect') {
      const items = (Array.isArray(value) ? value : []).map((item) => String(item).trim()).filter(Boolean)
      if (!items.length) continue
      assign(payload, field, items)
      continue
    }
    if (field.type === 'number') {
      const num = Number(value ?? 0)
      if (!Number.isFinite(num) || num === 0) continue
      assign(payload, field, num)
      continue
    }
    const text = typeof value === 'string' ? value.trim() : ''
    if (text === '') {
      // match_* / provision_* 是"清空即恢复默认/不启用"的策略字段：
      // 必须显式写回空串，否则后端把缺键当成"保持原值"，出现"清了却还在生效"。
      if (field.policy && (field.key.startsWith('match_') || field.key.startsWith('provision_'))) {
        assign(payload, field, '')
      }
      continue
    }
    assign(payload, field, text)
  }
  return payload
}

function assign(payload: ProviderPayload, field: ProviderField, value: unknown) {
  const bucket = field.policy ? payload.policy! : payload.config!
  bucket[field.key] = value
}
