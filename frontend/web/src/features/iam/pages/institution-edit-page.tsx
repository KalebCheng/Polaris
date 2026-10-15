import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { Eye } from 'lucide-react'
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Input,
  MultiSelect,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
  Switch,
  Textarea,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { FormSection } from '@/shared/components/form-section'
import { FormWizard, type WizardStep } from '@/shared/components/form-wizard'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { UploadOrURL } from '@/shared/components/upload-or-url'
import { Field } from '../components/field'
import { LoginPagePreview } from '../components/login-page-preview'
import { PASSWORD_OPTIONS, PASSWORD_TYPES } from '../lib/password-policy'
import { applicationApi, institutionApi } from '../api'
import { useMutation } from '../hooks/use-mutation'
import type { InstitutionInput } from '@/shared/api/types'

interface Form {
  name: string
  display_name: string
  logo: string
  favicon: string
  website_url: string
  description: string
  password_type: string
  password_options: string[]
  password_salt: string
  password_expire_days: number
  disable_signin: boolean
  use_email_as_username: boolean
  mfa_policy: string
  mfa_remember_hours: number
}

const emptyForm: Form = {
  name: '',
  display_name: '',
  logo: '',
  favicon: '',
  website_url: '',
  description: '',
  password_type: 'bcrypt',
  password_options: ['AtLeast8'],
  password_salt: '',
  password_expire_days: 0,
  disable_signin: false,
  use_email_as_username: false,
  mfa_policy: 'optional',
  mfa_remember_hours: 0,
}

/** 两步：先「基本信息与品牌」（必要），再「密码与安全」（策略）。 */
const STEP_KEYS = ['basic', 'security'] as const

/**
 * 组织「新建 / 编辑」页。
 * 采用与「应用编辑」一致的**分步向导**（顶部步骤条 + 分区表单 + 右侧登录页实时预览 + 吸底操作栏），
 * 见 docs/design-language.md §4：字段多按语义分步、成组呈现，品牌类配置配实时预览、改哪看哪。
 */
export default function InstitutionEditPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id = '' } = useParams()
  const orgID = Number(id)
  const isEdit = Number.isFinite(orgID) && orgID > 0
  const { pending, run } = useMutation()

  const [current, setCurrent] = useState(0)
  const [loading, setLoading] = useState(isEdit)
  const [form, setForm] = useState<Form>(emptyForm)
  const [hasSalt, setHasSalt] = useState(false)
  // 登录卡片图标取「当前应用」的 icon（组织默认应用优先，其次首个应用）
  const [cardLogo, setCardLogo] = useState('')
  const [confirmDiscard, setConfirmDiscard] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)

  const baseline = useRef<string>(JSON.stringify(emptyForm))
  const dirty = !loading && JSON.stringify(form) !== baseline.current

  const steps: WizardStep[] = [
    {
      key: STEP_KEYS[0],
      titleKey: 'institutions.stepBasic',
      descriptionKey: 'institutions.stepBasicDesc',
    },
    {
      key: STEP_KEYS[1],
      titleKey: 'institutions.stepSecurity',
      descriptionKey: 'institutions.stepSecurityDesc',
    },
  ]

  useEffect(() => {
    if (!isEdit) {
      baseline.current = JSON.stringify(emptyForm)
      return
    }
    let cancelled = false
    setLoading(true)
    void (async () => {
      try {
        const org = await institutionApi.get(orgID)
        if (cancelled) return
        const next: Form = {
          name: org.name,
          display_name: org.display_name ?? '',
          logo: org.logo ?? '',
          favicon: org.favicon ?? '',
          website_url: org.website_url ?? '',
          description: org.description ?? '',
          password_type: org.password_type || 'bcrypt',
          password_options: org.password_options ?? [],
          password_salt: '',
          password_expire_days: org.password_expire_days ?? 0,
          disable_signin: org.disable_signin,
          use_email_as_username: org.use_email_as_username,
          mfa_policy: org.mfa_policy || 'optional',
          mfa_remember_hours: org.mfa_remember_hours ?? 0,
        }
        setForm(next)
        baseline.current = JSON.stringify(next)
        setHasSalt(org.has_password_salt)
        // 应用是可选资源：取不到时保持为空（预览卡片不显示图标）
        try {
          const apps = await applicationApi.list({ institution: String(orgID), size: 200 })
          const preferred = apps.items.find((item) => item.name === org.default_application) ?? apps.items[0]
          setCardLogo(preferred?.logo ?? '')
        } catch {
          setCardLogo('')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [orgID, isEdit])

  const set = <K extends keyof Form>(key: K, value: Form[K]) => setForm((prev) => ({ ...prev, [key]: value }))

  const submit = async () => {
    const payload: InstitutionInput = {
      name: form.name,
      display_name: form.display_name,
      logo: form.logo,
      favicon: form.favicon,
      website_url: form.website_url,
      description: form.description,
      password_type: form.password_type,
      password_options: form.password_options,
      password_expire_days: Number(form.password_expire_days) || 0,
      disable_signin: form.disable_signin,
      use_email_as_username: form.use_email_as_username,
      mfa_policy: form.mfa_policy,
      mfa_remember_hours: Number(form.mfa_remember_hours) || 0,
    }
    // 密码盐为 write-only：仅在新输入时提交，留空表示保持既有值
    if (form.password_salt.trim()) {
      payload.password_salt = form.password_salt.trim()
    }
    const ok = await run(() =>
      isEdit ? institutionApi.update(orgID, payload) : institutionApi.create(payload),
    )
    if (ok) {
      baseline.current = JSON.stringify(form)
      navigate('/institutions')
    }
  }

  const requestCancel = () => {
    if (dirty) {
      setConfirmDiscard(true)
      return
    }
    navigate('/institutions')
  }

  return (
    <PageShell
      title={isEdit ? t('institutions.editTitle') : t('institutions.createTitle')}
      subtitleKey="institutions.subtitle"
      actions={
        <Button variant="outline" onClick={() => setPreviewOpen(true)}>
          <Eye className="size-4" /> {t('institutions.previewLogin')}
        </Button>
      }
    >
      {loading ? (
        <div className="space-y-3">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : (
        <FormWizard
          steps={steps}
          current={current}
          onStepChange={setCurrent}
          onFinish={() => void submit()}
          finishLabelKey="common.save"
          onCancel={requestCancel}
          pending={pending}
          canProceed={form.name.trim() !== ''}
        >
          {current === 0 ? (
            <>
              <FormSection titleKey="institutions.sectionBasic" descriptionKey="institutions.sectionBasicDesc">
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field
                    id="org-name"
                    labelKey="institutions.identifier"
                    hint={t('institutions.identifierHint')}
                  >
                    <Input
                      id="org-name"
                      value={form.name}
                      disabled={isEdit}
                      onChange={(event) => set('name', event.target.value)}
                    />
                  </Field>
                  <Field id="org-display" labelKey="institutions.name">
                    <Input
                      id="org-display"
                      value={form.display_name}
                      onChange={(event) => set('display_name', event.target.value)}
                    />
                  </Field>
                  <Field id="org-website" labelKey="institutions.website">
                    <Input
                      id="org-website"
                      value={form.website_url}
                      placeholder="https://"
                      onChange={(event) => set('website_url', event.target.value)}
                    />
                  </Field>
                  <Field id="org-description" labelKey="institutions.description">
                    <Textarea
                      id="org-description"
                      rows={3}
                      value={form.description}
                      onChange={(event) => set('description', event.target.value)}
                    />
                  </Field>
                </div>
              </FormSection>

              <FormSection titleKey="institutions.sectionBrand" descriptionKey="institutions.sectionBrandDesc">
                <div className="space-y-4">
                  <Field id="org-logo" labelKey="institutions.logo">
                    <UploadOrURL
                      id="org-logo"
                      value={form.logo}
                      onChange={(value) => set('logo', value)}
                      category="logos"
                      institution={isEdit ? String(orgID) : undefined}
                    />
                  </Field>
                  <Field id="org-favicon" labelKey="institutions.favicon">
                    <UploadOrURL
                      id="org-favicon"
                      value={form.favicon}
                      onChange={(value) => set('favicon', value)}
                      category="favicons"
                      institution={isEdit ? String(orgID) : undefined}
                    />
                  </Field>
                </div>
              </FormSection>
            </>
          ) : (
            <>
              <FormSection titleKey="institutions.sectionPassword" descriptionKey="institutions.sectionPasswordDesc">
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field id="org-pwd-type" labelKey="institutions.passwordType">
                    <Select value={form.password_type} onValueChange={(value) => set('password_type', value)}>
                      <SelectTrigger id="org-pwd-type">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {PASSWORD_TYPES.map((type) => (
                          <SelectItem key={type} value={type}>
                            {type}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field id="org-expire" labelKey="institutions.passwordExpireDays">
                    <Input
                      id="org-expire"
                      type="number"
                      min={0}
                      value={form.password_expire_days}
                      onChange={(event) => set('password_expire_days', Number(event.target.value))}
                    />
                  </Field>
                  <Field
                    id="org-policy"
                    labelKey="institutions.passwordOptions"
                    hint={t('institutions.passwordOptionsHint')}
                  >
                    <MultiSelect
                      options={PASSWORD_OPTIONS.map((option) => ({ value: option.value, label: t(option.labelKey) }))}
                      value={form.password_options}
                      onChange={(next) => set('password_options', next)}
                      placeholder={t('institutions.passwordOptions')}
                    />
                  </Field>
                  <Field
                    id="org-salt"
                    labelKey="institutions.passwordSalt"
                    hint={hasSalt ? t('institutions.passwordSaltConfigured') : t('institutions.passwordSaltHint')}
                  >
                    <Input
                      id="org-salt"
                      type="password"
                      autoComplete="new-password"
                      value={form.password_salt}
                      placeholder={t('institutions.passwordSaltPlaceholder')}
                      onChange={(event) => set('password_salt', event.target.value)}
                    />
                  </Field>
                </div>
              </FormSection>

              <FormSection titleKey="institutions.sectionMfa" descriptionKey="institutions.sectionMfaDesc">
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field id="org-mfa-policy" labelKey="institutions.mfaPolicy">
                    <Select value={form.mfa_policy} onValueChange={(value) => set('mfa_policy', value)}>
                      <SelectTrigger id="org-mfa-policy">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="optional">{t('institutions.mfaPolicyOptional')}</SelectItem>
                        <SelectItem value="required">{t('institutions.mfaPolicyRequired')}</SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field
                    id="org-mfa-remember"
                    labelKey="institutions.mfaRememberHours"
                    hint={t('institutions.mfaRememberHint')}
                  >
                    <Input
                      id="org-mfa-remember"
                      type="number"
                      min={0}
                      value={form.mfa_remember_hours}
                      onChange={(event) => set('mfa_remember_hours', Number(event.target.value))}
                    />
                  </Field>
                </div>
              </FormSection>

              <FormSection
                titleKey="institutions.sectionAdvanced"
                descriptionKey="institutions.sectionAdvancedDesc"
                collapsible
              >
                <div className="grid gap-4 sm:grid-cols-2">
                  <label className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2.5 text-sm">
                    <span>{t('institutions.disableSignin')}</span>
                    <Switch checked={form.disable_signin} onCheckedChange={(value) => set('disable_signin', value)} />
                  </label>
                  <label className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2.5 text-sm">
                    <span>{t('institutions.useEmailAsUsername')}</span>
                    <Switch
                      checked={form.use_email_as_username}
                      onCheckedChange={(value) => set('use_email_as_username', value)}
                    />
                  </label>
                </div>
              </FormSection>
            </>
          )}
        </FormWizard>
      )}

      {/* 登录页预览改为按需弹窗：不占常驻栏，表单可占满整宽 */}
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent
          className="sm:max-w-4xl"
          innerClassName="p-0 gap-0"
          // 预览是静态示意：不要让弹窗把焦点移进预览里的输入框（会显示焦点环）
          onOpenAutoFocus={(event) => event.preventDefault()}
        >
          <DialogHeader className="border-b px-6 py-4">
            <DialogTitle>{t('institutions.previewTitle')}</DialogTitle>
            <DialogDescription>{t('institutions.previewHint')}</DialogDescription>
          </DialogHeader>
          <LoginPagePreview
            logo={form.logo}
            displayName={form.display_name}
            name={form.name}
            description={form.description}
            cardLogo={cardLogo || form.logo}
          />
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirmDiscard}
        onOpenChange={setConfirmDiscard}
        titleKey="applications.unsavedTitle"
        descriptionKey="applications.unsavedBody"
        confirmLabelKey="common.discard"
        onConfirm={() => {
          setConfirmDiscard(false)
          navigate('/institutions')
        }}
      />
    </PageShell>
  )
}
