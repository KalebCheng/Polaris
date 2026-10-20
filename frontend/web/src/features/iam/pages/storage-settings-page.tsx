import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Check } from 'lucide-react'
import { Button, Input, Skeleton } from '@/shared/ui'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { FormSection } from '@/shared/components/form-section'
import { LoadError } from '@/shared/components/load-error'
import { ProviderTile } from '@/shared/components/provider-icon'
import { SwitchField } from '@/shared/components/switch-field'
import { describeError } from '@/shared/lib/errors'
import { formatDateTime } from '@/shared/lib/format'
import { cn } from '@/shared/lib/utils'
import type { ProviderCatalog, StorageSettings } from '@/shared/api/types'
import { providerApi, storageApi } from '../api'
import { ConnectionCard, type ConnectionCardView } from '../components/connection-card'
import { FormField } from '../components/field-row'
import { usePageHeaderActions } from '../components/page-header-actions'
import { useMutation } from '../hooks/use-mutation'

/** 提交载荷：与 `storage_settings` 表列一一对应；`secret_key` 为 write-only（留空 = 保持原值） */
interface Form {
  type: string
  root_path: string
  public_base: string
  endpoint: string
  bucket: string
  region: string
  access_key: string
  secret_key: string
  path_prefix: string
  domain: string
  path_style: boolean
}

const emptyForm: Form = {
  type: '',
  root_path: '',
  public_base: '',
  endpoint: '',
  bucket: '',
  region: '',
  access_key: '',
  secret_key: '',
  path_prefix: '',
  domain: '',
  path_style: true,
}

/** 响应 → 表单：密钥绝不回显（只回 has_secret），因此这里恒为空串 */
function formOf(settings: StorageSettings): Form {
  return {
    type: settings.type ?? '',
    root_path: settings.root_path ?? '',
    public_base: settings.public_base ?? '',
    endpoint: settings.endpoint ?? '',
    bucket: settings.bucket ?? '',
    region: settings.region ?? '',
    access_key: settings.access_key ?? '',
    secret_key: '',
    path_prefix: settings.path_prefix ?? '',
    domain: settings.domain ?? '',
    path_style: settings.path_style ?? true,
  }
}

/** 连接字段的键（与实体列同名，避免"界面字段名"与"落库列"两套说法） */
type ConnectionKey =
  | 'root_path'
  | 'public_base'
  | 'endpoint'
  | 'bucket'
  | 'region'
  | 'access_key'
  | 'path_prefix'
  | 'domain'

interface FieldSpec {
  key: ConnectionKey
  labelKey: string
  hintKey?: string
  placeholder?: string
  /** 端点 / 根目录这类长值占整行 */
  wide?: boolean
}

/**
 * 连接字段：**只列运行时会真的读到的字段**（`StorageService.platformStore` → `pkg/storage`）。
 * 本地模式读 root_path + public_base，对象存储读 endpoint / bucket / region / access_key /
 * path_prefix / domain / path_style——界面上出现的每一项都对应一处真实读取点（AGENTS §1「配置即生效」）。
 */
const LOCAL_FIELDS: FieldSpec[] = [
  {
    key: 'root_path',
    labelKey: 'storage.rootPath',
    hintKey: 'storage.rootPathHint',
    placeholder: '/data/polaris/storage',
    wide: true,
  },
  {
    key: 'public_base',
    labelKey: 'storage.publicBase',
    hintKey: 'storage.publicBaseHint',
    placeholder: 'https://iam.example.com',
    wide: true,
  },
]

const OBJECT_FIELDS: FieldSpec[] = [
  {
    key: 'endpoint',
    labelKey: 'storage.endpoint',
    hintKey: 'storage.endpointHint',
    placeholder: 'https://s3.example.com',
    wide: true,
  },
  { key: 'bucket', labelKey: 'storage.bucket' },
  { key: 'region', labelKey: 'storage.region', hintKey: 'storage.regionHint', placeholder: 'us-east-1' },
  { key: 'access_key', labelKey: 'storage.accessKey', hintKey: 'storage.accessKeyHint' },
  { key: 'path_prefix', labelKey: 'storage.pathPrefix', placeholder: 'polaris/' },
  { key: 'domain', labelKey: 'storage.domain', hintKey: 'storage.domainHint', wide: true },
]

function connectionFields(type: string): FieldSpec[] {
  if (type === 'local') return LOCAL_FIELDS
  if (type === 's3' || type === 'minio') return OBJECT_FIELDS
  return []
}

/** 对象存储（需密钥与 PathStyle）；本地磁盘不需要 */
function isObjectStorage(type: string): boolean {
  return type === 's3' || type === 'minio'
}

interface StorageTypeOption {
  key: string
  name: string
  description: string
  icon: string
}

/**
 * 类型清单（图标 / 名称 / 说明）来自**提供商目录**（`GET /providers/catalog` 的 storage 类别）——
 * 与组织级「存储提供商」同一份声明，这里不再抄一份类型表。
 * 目录不可用时退回内置三种（类型集合本身由后端 `isSupportedStorageType` 固定），保证页面可用。
 */
function storageTypeOptions(
  catalog: ProviderCatalog | null,
  t: (key: string) => string,
): StorageTypeOption[] {
  const types = catalog?.categories.find((group) => group.key === 'storage')?.types ?? []
  if (types.length > 0) {
    return types.map((type) => ({
      key: type.key,
      name: type.name,
      description: type.description,
      icon: type.icon,
    }))
  }
  return [
    { key: 'local', name: t('storage.typeLocal'), description: t('storage.typeLocalDesc'), icon: 'disk' },
    { key: 's3', name: t('storage.typeS3'), description: t('storage.typeS3Desc'), icon: 's3' },
    { key: 'minio', name: t('storage.typeMinio'), description: t('storage.typeMinioDesc'), icon: 'minio' },
  ]
}

/**
 * 系统设置 › 存储设置（平台级）：平台默认对象存储。
 *
 * 与组织级「存储提供商」的**同一套交互**：一张实例卡片（类型图标 / 状态药丸 / 连接信息 /
 * 行内「测试连通性 / 修改配置 / 清除配置」），类型与图标取自提供商目录；
 * 差别只在它是**单例且跨组织**——组织未单独配置存储提供商时，平台托管的文件（组织 / 应用 Logo、
 * 头像等）回落于此，所以卡片上还要说清"谁在用它"。
 *
 * 编辑用**页内分区表单**（字段彼此无依赖，类型决定字段集合），不切步骤、不开弹窗：
 * 一屏就能改完，改完即见卡片回到新状态（"改哪看哪"）。
 */
export default function StorageSettingsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [settings, setSettings] = useState<StorageSettings | null>(null)
  const [catalog, setCatalog] = useState<ProviderCatalog | null>(null)
  const [loadError, setLoadError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<Form>(emptyForm)
  const [confirmClear, setConfirmClear] = useState(false)
  const save = useMutation()
  const test = useMutation()

  const load = useCallback(async () => {
    setLoadError('')
    try {
      setSettings(await storageApi.getSettings())
    } catch (error) {
      // 读不到 ≠ 没配置：请求失败若静默渲染成「未配置」，管理员会以为平台存储已经停摆
      setSettings(null)
      setLoadError(describeError(error, t))
    } finally {
      setLoaded(true)
    }
  }, [t])

  useEffect(() => {
    void load()
  }, [load])

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

  const typeOptions = useMemo(() => storageTypeOptions(catalog, t), [catalog, t])
  const configured = Boolean(settings?.type)
  const option = typeOptions.find((item) => item.key === settings?.type)

  const setField = (key: keyof Form, value: string | boolean) =>
    setForm((prev) => ({ ...prev, [key]: value }))

  const startEdit = () => {
    if (!settings) return
    setForm(formOf(settings))
    setEditing(true)
  }

  /**
   * 切换类型：只保留新旧类型**都会读**的字段。
   * 不同类型的字段集合不同构，把旧值带过去就会写出半配置（例如切到本地磁盘却还留着对象存储的
   * Bucket 与 PathStyle）——那正是"配了但不生效"的温床。
   */
  const switchType = (next: string) => {
    setForm((prev) => {
      const kept: Form = {
        ...emptyForm,
        type: next,
        secret_key: prev.secret_key,
        path_style: prev.path_style,
      }
      for (const field of connectionFields(next)) kept[field.key] = prev[field.key]
      return kept
    })
  }

  const submit = async () => {
    const ok = await save.run(() => storageApi.updateSettings({ ...form }), 'storage.saved')
    if (!ok) return
    setEditing(false)
    await load()
  }

  const clear = async () => {
    // type 为空 = 显式「未配置」：后端整行清零（clear 语义由 StorageService.UpdateSettings 保证）
    const ok = await save.run(() => storageApi.updateSettings({ type: '' }), 'storage.cleared')
    if (!ok) return
    setConfirmClear(false)
    setEditing(false)
    await load()
  }

  const locate = (): string => {
    if (!settings) return ''
    if (settings.type === 'local') return settings.root_path || '-'
    return [settings.endpoint, settings.bucket].filter(Boolean).join(' / ') || '-'
  }

  const secretState = (): string => {
    if (!settings?.type) return t('storage.statusUnset')
    if (!isObjectStorage(settings.type)) return t('storage.noSecretNeeded')
    return settings.has_secret ? t('storage.secretConfigured') : t('storage.secretMissing')
  }

  const connectionHint = (): string => {
    if (!settings?.type) return t('storage.unsetConnectionHint')
    if (settings.type === 'local') return t('storage.localHint', { path: settings.root_path || '-' })
    return t('storage.objectHint', {
      bucket: settings.bucket || '-',
      prefix: settings.path_prefix || '-',
    })
  }

  const view: ConnectionCardView | null = settings
    ? {
        // 未配置时不把「未配置」写三遍（标题 / 标签 / 药丸各一次）：标题说清"这是什么状态"
        name: configured ? (option?.name ?? settings.type) : t('storage.unsetTitle'),
        subtitle: settings.type || 'unset',
        icon: { key: option?.icon ?? 'storage' },
        type: settings.type || 'storage',
        tags: configured ? [t('storage.platformDefault'), settings.type] : [t('storage.platformDefault')],
        meta: configured
          ? [locate(), secretState(), t('storage.updatedAt', { value: formatDateTime(settings.updated_at) })]
          : [t('storage.unsetHint')],
        status: {
          tone: configured ? 'success' : 'neutral',
          label: configured ? t('storage.statusActive') : t('storage.statusUnset'),
        },
        sections: [
          {
            label: t('storage.connectionSection'),
            hint: connectionHint(),
            links: configured
              ? [
                  {
                    label: test.pending ? t('storage.testing') : t('storage.test'),
                    onClick: () => void test.run(() => storageApi.testSettings(), 'storage.testSuccess'),
                  },
                ]
              : [],
          },
          {
            label: t('storage.fallbackSection'),
            hint: t('storage.fallbackHint'),
            links: [
              { label: t('storage.orgStorageLink'), onClick: () => navigate('/providers?category=storage') },
            ],
          },
        ],
        onEdit: startEdit,
        // 未配置时没有"修改/清除"可言，只有"配置"
        onDelete: configured ? () => setConfirmClear(true) : undefined,
        editLabelKey: configured ? 'storage.editConfig' : 'storage.configure',
        // 未配置时「配置存储」是页面唯一的主操作，用主色按钮（描边按钮在白色卡片上几乎看不见）
        editVariant: configured ? 'ghost' : 'default',
        removeLabelKey: 'storage.clearConfig',
      }
    : null

  // 页头操作：编辑态的提交动作跟随页面表单状态，由页面渲染后 portal 到布局页头
  const headerActions = usePageHeaderActions(
    // 视图态的动作都收敛在实例卡片上（配置 / 修改配置 / 清除配置），页头只在编辑态承载提交动作
    editing ? (
      <>
        <Button variant="outline" disabled={save.pending} onClick={() => setEditing(false)}>
          {t('common.cancel')}
        </Button>
        <Button disabled={save.pending || !form.type} onClick={() => void submit()}>
          {t('common.save')}
        </Button>
      </>
    ) : null,
  )

  return (
    <>
      {headerActions}
      {!loaded ? (
        <Skeleton className="h-56 w-full" />
      ) : loadError ? (
        <LoadError onRetry={() => void load()} detail={loadError} />
      ) : editing ? (
        <div className="space-y-5">
          <FormSection titleKey="storage.typeSection" descriptionKey="storage.typeSectionHint">
            <div role="radiogroup" aria-label={t('storage.type')} className="grid gap-3 sm:grid-cols-3">
              {typeOptions.map((item) => {
                const active = form.type === item.key
                return (
                  <Button
                    key={item.key}
                    type="button"
                    variant="outline"
                    role="radio"
                    aria-checked={active}
                    onClick={() => switchType(item.key)}
                    className={cn(
                      'h-auto w-full items-start justify-start gap-3 rounded-2xl px-3.5 py-3 text-left whitespace-normal',
                      active && 'border-primary/60 bg-primary/5',
                    )}
                  >
                    <ProviderTile iconKey={item.icon} tileSize={36} />
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-1.5 text-[13px] font-medium">
                        {item.name}
                        {active ? <Check className="text-primary size-3.5 shrink-0" /> : null}
                      </span>
                      <span className="text-muted-foreground mt-0.5 block text-[11px] leading-relaxed">
                        {item.description}
                      </span>
                    </span>
                  </Button>
                )
              })}
            </div>
          </FormSection>

          {form.type ? (
            <FormSection titleKey="storage.connectionSection" descriptionKey="storage.connectionSectionHint">
              <div className="grid gap-4 sm:grid-cols-2">
                {connectionFields(form.type).map((field) => (
                  <FormField
                    key={field.key}
                    id={`storage-${field.key}`}
                    labelKey={field.labelKey}
                    hint={field.hintKey ? t(field.hintKey) : undefined}
                    className={cn('space-y-2', field.wide && 'sm:col-span-2')}
                  >
                    <Input
                      id={`storage-${field.key}`}
                      value={form[field.key]}
                      placeholder={field.placeholder}
                      onChange={(event) => setField(field.key, event.target.value)}
                    />
                  </FormField>
                ))}

                {isObjectStorage(form.type) ? (
                  <>
                    <FormField
                      id="storage-secret_key"
                      labelKey="storage.secretKey"
                      hint={settings?.has_secret ? t('storage.secretConfigured') : t('storage.secretMissing')}
                      className="space-y-2"
                    >
                      <Input
                        id="storage-secret_key"
                        type="password"
                        autoComplete="new-password"
                        value={form.secret_key}
                        onChange={(event) => setField('secret_key', event.target.value)}
                      />
                    </FormField>
                    <SwitchField
                      className="self-start sm:col-span-2"
                      labelKey="storage.pathStyle"
                      descriptionKey="storage.pathStyleHint"
                      checked={form.path_style}
                      onCheckedChange={(checked) => setField('path_style', checked)}
                    />
                  </>
                ) : null}
              </div>
            </FormSection>
          ) : null}
        </div>
      ) : view ? (
        <ConnectionCard view={view} />
      ) : null}

      <ConfirmDialog
        open={confirmClear}
        onOpenChange={setConfirmClear}
        titleKey="storage.clearConfirmTitle"
        descriptionKey="storage.clearConfirmBody"
        loading={save.pending}
        onConfirm={() => void clear()}
      />
    </>
  )
}
