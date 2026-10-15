import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Card,
  CardContent,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
  Switch,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { FormField } from '../components/field-row'
import { storageApi } from '../api'
import { useMutation } from '../hooks/use-mutation'

/** Radix Select 不接受空串取值：用哨兵映射"未配置"。 */
const NONE = '__none__'

interface Form {
  type: string
  endpoint: string
  region: string
  bucket: string
  access_key: string
  secret_key: string
  path_prefix: string
  domain: string
  root_path: string
  public_base: string
  path_style: boolean
}

const emptyForm: Form = {
  type: '',
  endpoint: '',
  region: '',
  bucket: '',
  access_key: '',
  secret_key: '',
  path_prefix: '',
  domain: '',
  root_path: '',
  public_base: '',
  path_style: true,
}

/**
 * 系统级默认对象存储设置（平台管理员）。
 * 组织未单独配置 Storage 提供商时，平台托管的文件（组织/应用 Logo、头像等）回落到此存储。
 */
export default function StorageSettingsPage() {
  const { t } = useTranslation()
  const [form, setForm] = useState<Form>(emptyForm)
  const [hasSecret, setHasSecret] = useState(false)
  const [loading, setLoading] = useState(true)
  const save = useMutation()
  const test = useMutation()

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const settings = await storageApi.getSettings()
        if (cancelled) return
        setForm({
          type: settings.type ?? '',
          endpoint: settings.endpoint ?? '',
          region: settings.region ?? '',
          bucket: settings.bucket ?? '',
          access_key: settings.access_key ?? '',
          secret_key: '',
          path_prefix: settings.path_prefix ?? '',
          domain: settings.domain ?? '',
          root_path: settings.root_path ?? '',
          public_base: settings.public_base ?? '',
          path_style: settings.path_style ?? true,
        })
        setHasSecret(settings.has_secret)
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const set = <K extends keyof Form>(key: K, value: Form[K]) => setForm((prev) => ({ ...prev, [key]: value }))

  const onSave = async () => {
    const payload: Form = form
    const ok = await save.run(() => storageApi.updateSettings(payload), 'storage.saved')
    if (ok && form.secret_key.trim()) setHasSecret(true)
    if (ok) set('secret_key', '')
  }

  const isObject = form.type === 's3' || form.type === 'minio'

  return (
    <PageShell
      titleKey="storage.title"
      subtitleKey="storage.subtitle"
      actions={
        <>
          <Button
            variant="outline"
            disabled={test.pending || loading || form.type === ''}
            onClick={() => void test.run(() => storageApi.testSettings(), 'storage.testSuccess')}
          >
            {t('storage.test')}
          </Button>
          <Button disabled={save.pending || loading} onClick={() => void onSave()}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      {loading ? (
        <Skeleton className="h-72 w-full" />
      ) : (
        <Card className="max-w-3xl">
          <CardContent className="grid gap-4 sm:grid-cols-2">
            <FormField
              id="storage-type"
              labelKey="storage.type"
              hint={t('storage.typeHint')}
              className="space-y-2 sm:col-span-2"
            >
              <Select
                value={form.type === '' ? NONE : form.type}
                onValueChange={(value) => set('type', value === NONE ? '' : value)}
              >
                <SelectTrigger id="storage-type" className="sm:max-w-sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>{t('storage.typeNone')}</SelectItem>
                  <SelectItem value="local">{t('storage.typeLocal')}</SelectItem>
                  <SelectItem value="s3">{t('storage.typeS3')}</SelectItem>
                  <SelectItem value="minio">{t('storage.typeMinio')}</SelectItem>
                </SelectContent>
              </Select>
            </FormField>

            {form.type === 'local' ? (
              <>
                <FormField id="storage-root" labelKey="storage.rootPath" hint={t('storage.localHint')}>
                  <Input
                    id="storage-root"
                    value={form.root_path}
                    placeholder="/data/polaris/storage"
                    onChange={(e) => set('root_path', e.target.value)}
                  />
                </FormField>
                <FormField id="storage-public" labelKey="storage.publicBase" hint={t('storage.publicBaseHint')}>
                  <Input
                    id="storage-public"
                    value={form.public_base}
                    placeholder="https://iam.example.com"
                    onChange={(e) => set('public_base', e.target.value)}
                  />
                </FormField>
              </>
            ) : null}

            {isObject ? (
              <>
                <FormField id="storage-endpoint" labelKey="storage.endpoint" hint={t('storage.s3Hint')}>
                  <Input
                    id="storage-endpoint"
                    value={form.endpoint}
                    placeholder="https://s3.example.com"
                    onChange={(e) => set('endpoint', e.target.value)}
                  />
                </FormField>
                <FormField id="storage-bucket" labelKey="storage.bucket">
                  <Input id="storage-bucket" value={form.bucket} onChange={(e) => set('bucket', e.target.value)} />
                </FormField>
                <FormField id="storage-region" labelKey="storage.region">
                  <Input
                    id="storage-region"
                    value={form.region}
                    placeholder="us-east-1"
                    onChange={(e) => set('region', e.target.value)}
                  />
                </FormField>
                <FormField id="storage-prefix" labelKey="storage.pathPrefix">
                  <Input
                    id="storage-prefix"
                    value={form.path_prefix}
                    placeholder="polaris/"
                    onChange={(e) => set('path_prefix', e.target.value)}
                  />
                </FormField>
                <FormField id="storage-access" labelKey="storage.accessKey">
                  <Input id="storage-access" value={form.access_key} onChange={(e) => set('access_key', e.target.value)} />
                </FormField>
                <FormField
                  id="storage-secret"
                  labelKey="storage.secretKey"
                  hint={hasSecret ? t('storage.secretConfigured') : undefined}
                >
                  <Input
                    id="storage-secret"
                    type="password"
                    autoComplete="new-password"
                    value={form.secret_key}
                    onChange={(e) => set('secret_key', e.target.value)}
                  />
                </FormField>
                <FormField id="storage-domain" labelKey="storage.domain" hint={t('storage.domainHint')}>
                  <Input
                    id="storage-domain"
                    value={form.domain}
                    placeholder="https://cdn.example.com"
                    onChange={(e) => set('domain', e.target.value)}
                  />
                </FormField>
                <label className="flex items-center justify-between gap-3 self-end pb-2 text-sm">
                  <span className="flex flex-col">
                    <span>{t('storage.pathStyle')}</span>
                    <span className="text-muted-foreground text-xs">{t('storage.pathStyleHint')}</span>
                  </span>
                  <Switch checked={form.path_style} onCheckedChange={(value) => set('path_style', value)} />
                </label>
              </>
            ) : null}
          </CardContent>
        </Card>
      )}
    </PageShell>
  )
}
