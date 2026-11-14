import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronUp, ExternalLink, Plus, Trash2 } from 'lucide-react'
import {
  Badge,
  Button,
  Checkbox,
  Combobox,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  Input,
  MultiSelect,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Textarea,
  type SelectOption,
} from '@/shared/ui'
import { FormSection } from '@/shared/components/form-section'
import { FormWizard, type WizardStep } from '@/shared/components/form-wizard'
import { RepeatableList } from '@/shared/components/repeatable-list'
import { SigninMethodIcon } from '@/shared/components/signin-method-icon'
import { TagInput } from '@/shared/components/tag-input'
import { SwitchField } from '@/shared/components/switch-field'
import { UploadOrURL } from '@/shared/components/upload-or-url'
import type {
  Application,
  ApplicationAccess,
  Cert,
  InstitutionSigninMethod,
  JobTitle,
} from '@/shared/api/types'
import { ApplicationAccessEditor } from './application-access-editor'
import { TokenFieldEditor } from './token-field-editor'
import { Field } from './field'
import {
  applicationCodePattern,
  appTypeOptions,
  formToApplicationValidation,
  grantTypeOptions,
  invalidRedirectURI,
  scopeOptions,
  signupItemOptions,
  type ApplicationAccessFormValue,
  type ApplicationFormValue,
  type ScopeRow,
  type SignupFieldRow,
} from './application-form'
import { OptionChips } from './application-wizard-fields'

/** 四步向导的步骤定义（与步骤内容一一对应） */
const APP_STEPS: WizardStep[] = [
  { key: 'basic', titleKey: 'applications.stepBasic', descriptionKey: 'applications.stepBasicHint' },
  { key: 'signin', titleKey: 'applications.stepSignin', descriptionKey: 'applications.stepSigninHint' },
  { key: 'signup', titleKey: 'applications.stepSignup', descriptionKey: 'applications.stepSignupHint' },
  { key: 'client', titleKey: 'applications.stepClient', descriptionKey: 'applications.stepClientHint' },
  { key: 'security', titleKey: 'applications.stepSecurity', descriptionKey: 'applications.stepSecurityHint' },
  { key: 'access', titleKey: 'applications.stepAccess', descriptionKey: 'applications.stepAccessHint' },
]

const categoryOptions: SelectOption[] = [
  { value: 'default', label: 'default' },
  { value: 'web', label: 'web' },
  { value: 'native', label: 'native' },
  { value: 'desktop', label: 'desktop' },
]

const signupFieldLabel = (name: string) =>
  signupItemOptions.find((option) => option.value === name)?.label ?? name

function signinMethodSummary(method: InstitutionSigninMethod) {
  if (method.name === 'Password') {
    return method.identifiers?.length ? method.identifiers.join(' / ') : 'username'
  }
  if (method.name === 'Provider') {
    return method.provider_display_name || method.provider_name || method.display_name || 'Provider'
  }
  const channel = method.template_name || method.provider_display_name || method.provider_name || ''
  return [channel, method.ttl_seconds ? `${method.ttl_seconds}s` : ''].filter(Boolean).join(' · ')
}

interface ApplicationWizardProps {
  form: ApplicationFormValue
  onChange: (next: ApplicationFormValue) => void
  editing: Application | null
  /** 当前工作区组织（上传资源要落到该组织的存储提供商） */
  institution?: string
  /** 组织级登录方式（应用只勾选，不配置参数） */
  signinMethods: InstitutionSigninMethod[]
  /** 组织登录方式配置页（仅管理员可见） */
  signinMethodsHref?: string
  certs: Cert[]
  jobTitles: JobTitle[]
  pending?: boolean
  onFinish: () => void
  finishLabelKey: string
  onCancel?: () => void
  /** 访问授权（谁能使用该应用）：独立于应用主表单，走 PUT /applications/:id/access */
  access: ApplicationAccessFormValue
  onAccessChange: (next: ApplicationAccessFormValue) => void
  /** 已保存的授权详情（含展示名与"已失效"标记），用于把已选主体显示成人类可读的名称 */
  accessDetail?: ApplicationAccess | null
}

/**
 * 应用新建 / 编辑向导：四步（基本信息 → 登录方式 → 注册与自助服务 → 客户端与安全）。
 * 创建与编辑复用同一组件；登录页预览改为页面上的按需弹窗（见 application-edit-page）。
 */
export function ApplicationWizard({
  form,
  onChange,
  editing,
  institution,
  signinMethods,
  signinMethodsHref,
  certs,
  jobTitles,
  pending,
  onFinish,
  finishLabelKey,
  onCancel,
  access,
  onAccessChange,
  accessDetail,
}: ApplicationWizardProps) {
  const { t } = useTranslation()

  const set = <K extends keyof ApplicationFormValue>(key: K, value: ApplicationFormValue[K]) =>
    onChange({ ...form, [key]: value })

  const certOptions: SelectOption[] = certs.map((cert) => ({ value: String(cert.id), label: cert.name }))
  const jobTitleOptions: SelectOption[] = jobTitles.map((jobTitle) => ({
    value: String(jobTitle.id),
    label: jobTitle.display_name || jobTitle.name,
  }))

  const [current, setCurrent] = useState(0)

  const addScope = () =>
    onChange({ ...form, scopes: [...form.scopes, { name: '', displayName: '', description: '' }] })

  const updateScope = (index: number, patch: Partial<ScopeRow>) =>
    onChange({ ...form, scopes: form.scopes.map((row, i) => (i === index ? { ...row, ...patch } : row)) })

  const removeScope = (index: number) =>
    onChange({ ...form, scopes: form.scopes.filter((_, i) => i !== index) })

  const moveScope = (index: number, delta: number) => {
    const target = index + delta
    if (target < 0 || target >= form.scopes.length) return
    const next = [...form.scopes]
    const [row] = next.splice(index, 1)
    next.splice(target, 0, row)
    onChange({ ...form, scopes: next })
  }

  const stepGate = (step: number): boolean => {
    if (step === 0) return applicationCodePattern.test(form.code.trim())
    if (step === 1) return form.signin_method_ids.length > 0
    if (step === 3) return formToApplicationValidation(form).ok
    return true
  }

  /** 尚未添加的注册字段候选（内置字段始终已在列，故不在候选内） */
  const availableSignupFields = signupItemOptions.filter(
    (option) => !form.signup_fields.some((row) => row.name === option.value),
  )

  const addSignupField = (name: string) => {
    if (form.signup_fields.some((row) => row.name === name)) return
    onChange({ ...form, signup_fields: [...form.signup_fields, { name, required: false }] })
  }

  const updateSignupField = (name: string, patch: Partial<SignupFieldRow>) =>
    onChange({
      ...form,
      signup_fields: form.signup_fields.map((row) => (row.name === name ? { ...row, ...patch } : row)),
    })

  const removeSignupField = (name: string) =>
    onChange({ ...form, signup_fields: form.signup_fields.filter((row) => row.name !== name) })

  const moveSignupField = (index: number, delta: number) => {
    const target = index + delta
    if (target < 0 || target >= form.signup_fields.length) return
    const next = [...form.signup_fields]
    const [row] = next.splice(index, 1)
    next.splice(target, 0, row)
    onChange({ ...form, signup_fields: next })
  }

  const toggleSigninMethod = (id: number, checked: boolean) => {
    const ids = checked
      ? [...form.signin_method_ids, id]
      : form.signin_method_ids.filter((item) => item !== id)
    onChange({ ...form, signin_method_ids: Array.from(new Set(ids)) })
  }

  return (
    <FormWizard
      steps={APP_STEPS}
      current={current}
      onStepChange={setCurrent}
      onFinish={onFinish}
      finishLabelKey={finishLabelKey}
      onCancel={onCancel}
      pending={pending}
      canProceed={stepGate(current)}
    >
      {current === 0 ? (
        <>
          <FormSection
            titleKey="applications.sectionIdentity"
            descriptionKey="applications.sectionIdentityHint"
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="app-code" labelKey="applications.code" hint={t('applications.codeHint')}>
                <Input
                  id="app-code"
                  value={form.code}
                  disabled={Boolean(editing)}
                  onChange={(event) => set('code', event.target.value.toLowerCase())}
                  placeholder="portal"
                  required
                  aria-invalid={form.code.length > 0 && !applicationCodePattern.test(form.code.trim())}
                />
              </Field>
              <Field id="app-name" labelKey="applications.name" hint={t('applications.nameHint')}>
                <Input
                  id="app-name"
                  value={form.name}
                  onChange={(event) => set('name', event.target.value)}
                  placeholder={t('applications.namePlaceholder')}
                />
              </Field>
            </div>
            {form.code.length > 0 && !applicationCodePattern.test(form.code.trim()) ? (
              <p className="text-destructive text-xs">{t('applications.codeInvalid')}</p>
            ) : null}
          </FormSection>

          <FormSection titleKey="applications.sectionAssets" descriptionKey="applications.sectionAssetsHint">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="app-logo" labelKey="applications.logo" hint={t('applications.logoHint')}>
                <UploadOrURL
                  id="app-logo"
                  category="application"
                  value={form.logo}
                  onChange={(v) => set('logo', v)}
                  institution={institution}
                />
              </Field>
              <Field id="app-favicon" labelKey="applications.favicon" hint={t('applications.faviconHint')}>
                <UploadOrURL
                  id="app-favicon"
                  institution={institution}
                  category="favicons"
                  value={form.favicon}
                  onChange={(v) => set('favicon', v)}
                />
              </Field>
              <Field id="app-appTitle" labelKey="applications.appTitle">
                <Input
                  id="app-appTitle"
                  value={form.title}
                  onChange={(event) => set('title', event.target.value)}
                />
              </Field>
              <Field id="app-homepage" labelKey="applications.homepage">
                <Input
                  id="app-homepage"
                  value={form.homepage_url}
                  onChange={(event) => set('homepage_url', event.target.value)}
                  placeholder="https://"
                />
              </Field>
            </div>
          </FormSection>

          <FormSection
            titleKey="applications.sectionTheme"
            descriptionKey="applications.sectionThemeHint"
            collapsible
          >
            <div className="space-y-4">
              <SwitchField
                labelKey="applications.customTheme"
                descriptionKey="applications.desc.customTheme"
                checked={form.theme_enabled}
                onCheckedChange={(checked) => set('theme_enabled', checked)}
              />
              {form.theme_enabled ? (
                <div className="grid gap-4 sm:grid-cols-3">
                  <Field
                    id="app-color"
                    labelKey="applications.colorPrimary"
                    hint={t('applications.colorPrimaryHint')}
                  >
                    <Input
                      id="app-color"
                      value={form.color_primary}
                      onChange={(event) => set('color_primary', event.target.value)}
                      placeholder="#5B53E8"
                    />
                  </Field>
                  <Field id="app-radius" labelKey="applications.borderRadius">
                    <Input
                      id="app-radius"
                      type="number"
                      min={0}
                      value={form.border_radius}
                      onChange={(event) => set('border_radius', Number(event.target.value))}
                    />
                  </Field>
                  <SwitchField
                    labelKey="applications.compact"
                    descriptionKey="applications.desc.compact"
                    checked={form.is_compact}
                    onCheckedChange={(checked) => set('is_compact', checked)}
                    className="sm:mt-6"
                  />
                </div>
              ) : null}
            </div>
          </FormSection>

          <FormSection titleKey="applications.sectionType" descriptionKey="applications.sectionTypeHint">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="app-type" labelKey="applications.type">
                <OptionChips options={appTypeOptions} value={form.type} onChange={(v) => set('type', v)} />
              </Field>
              <Field id="app-category" labelKey="applications.category">
                <OptionChips
                  options={categoryOptions}
                  value={form.category}
                  onChange={(v) => set('category', v)}
                />
              </Field>
              <Field id="app-state" labelKey="applications.state" hint={t('applications.stateHint')}>
                <Select value={form.state} onValueChange={(value) => set('state', value)}>
                  <SelectTrigger id="app-state">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="active">{t('common.enabled')}</SelectItem>
                    <SelectItem value="disabled">{t('common.disabled')}</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field id="app-tags" labelKey="applications.tags" hint={t('applications.tagsHint')}>
                <TagInput
                  id="app-tags"
                  value={form.tags}
                  onChange={(next) => set('tags', next)}
                  placeholder={t('applications.tagsPlaceholder')}
                />
              </Field>
              <Field id="app-desc" labelKey="applications.description" className="space-y-2 sm:col-span-2">
                <Textarea
                  id="app-desc"
                  rows={3}
                  value={form.description}
                  onChange={(event) => set('description', event.target.value)}
                />
              </Field>
            </div>
          </FormSection>
        </>
      ) : null}

      {current === 1 ? (
        <>
          <FormSection
            titleKey="applications.sectionMethods"
            descriptionKey="applications.sectionMethodsHint"
          >
            {signinMethods.length === 0 ? (
              <div className="space-y-3 text-sm">
                <p className="text-muted-foreground">{t('applications.noMethodsYet')}</p>
                {signinMethodsHref ? (
                  <Button asChild variant="outline">
                    <a href={signinMethodsHref}>
                      <ExternalLink className="size-4" />
                      {t('applications.configureSigninMethods')}
                    </a>
                  </Button>
                ) : null}
              </div>
            ) : (
              <div className="space-y-3">
                {signinMethods.map((method) => {
                  const checked = form.signin_method_ids.includes(method.id)
                  return (
                    <label
                      key={method.id}
                      className="hover:bg-muted/40 flex cursor-pointer items-start gap-3 rounded-lg border px-3.5 py-3 transition-colors"
                    >
                      <Checkbox
                        checked={checked}
                        onCheckedChange={(value) => toggleSigninMethod(method.id, Boolean(value))}
                        className="mt-0.5"
                      />
                      <SigninMethodIcon name={method.name} className="mt-0.5 size-4 shrink-0" />
                      <span className="min-w-0 flex-1">
                        <span className="flex flex-wrap items-center gap-2">
                          <span className="text-sm font-medium">{method.display_name || method.name}</span>
                          {method.mfa_enabled ? (
                            <Badge variant="secondary">
                              {method.mfa_required
                                ? t('applications.mfaRequiredBadge')
                                : t('applications.mfaEnabledBadge')}
                            </Badge>
                          ) : null}
                        </span>
                        <span className="text-muted-foreground mt-0.5 block truncate text-xs">
                          {signinMethodSummary(method)}
                        </span>
                      </span>
                      {method.state === 'disabled' ? (
                        <Badge variant="outline">{t('common.disabled')}</Badge>
                      ) : null}
                    </label>
                  )
                })}
                {signinMethodsHref ? (
                  <div className="pt-1">
                    <Button asChild variant="ghost" size="sm" className="px-0">
                      <a href={signinMethodsHref}>
                        <ExternalLink className="size-4" />
                        {t('applications.configureSigninMethods')}
                      </a>
                    </Button>
                  </div>
                ) : null}
              </div>
            )}
          </FormSection>

          <FormSection
            titleKey="applications.sectionSession"
            descriptionKey="applications.sectionSessionHint"
          >
            <Field
              id="app-max-sessions"
              labelKey="applications.maxSessions"
              hint={t('applications.maxSessionsHint')}
            >
              <Input
                id="app-max-sessions"
                type="number"
                min={0}
                value={form.max_sessions}
                onChange={(event) => set('max_sessions', Number(event.target.value))}
              />
            </Field>
          </FormSection>
        </>
      ) : null}

      {current === 2 ? (
        <>
          <FormSection titleKey="applications.sectionSignup" descriptionKey="applications.sectionSignupHint">
            <div className="space-y-3">
              <SwitchField
                labelKey="applications.selfSignup"
                descriptionKey="applications.desc.selfSignup"
                checked={form.self_signup_enabled}
                onCheckedChange={(checked) => set('self_signup_enabled', checked)}
              />
              {form.self_signup_enabled ? (
                <SwitchField
                  labelKey="applications.signupAutoSignin"
                  descriptionKey="applications.desc.signupAutoSignin"
                  checked={form.signup_auto_signin}
                  onCheckedChange={(checked) => set('signup_auto_signin', checked)}
                />
              ) : null}
            </div>
          </FormSection>

          {form.self_signup_enabled ? (
            <>
              <FormSection
                titleKey="applications.sectionSignupFields"
                descriptionKey="applications.sectionSignupFieldsHint"
              >
                <div className="space-y-3">
                  <div className="space-y-2">
                    {form.signup_fields.map((row, index) => (
                      <div key={row.name} className="flex items-center gap-2 rounded-lg border px-3 py-2">
                        <span className="min-w-0 flex-1 truncate text-[13px] font-medium">
                          {signupFieldLabel(row.name)}
                        </span>
                        {row.builtin ? (
                          <Badge variant="secondary" title={t('applications.signupFieldBuiltinHint')}>
                            {t('applications.signupFieldBuiltin')}
                          </Badge>
                        ) : null}
                        <label className="text-muted-foreground flex cursor-pointer items-center gap-1.5 text-xs">
                          <Checkbox
                            checked={row.required}
                            disabled={row.builtin}
                            onCheckedChange={(checked) =>
                              updateSignupField(row.name, { required: Boolean(checked) })
                            }
                            aria-label={t('applications.signupFieldRequired')}
                          />
                          {t('applications.signupFieldRequired')}
                        </label>
                        <div className="flex items-center gap-0.5">
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            disabled={index === 0}
                            aria-label={t('applications.moveUp')}
                            onClick={() => moveSignupField(index, -1)}
                          >
                            <ChevronUp className="size-4" />
                          </Button>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            disabled={index === form.signup_fields.length - 1}
                            aria-label={t('applications.moveDown')}
                            onClick={() => moveSignupField(index, 1)}
                          >
                            <ChevronDown className="size-4" />
                          </Button>
                        </div>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          disabled={row.builtin}
                          aria-label={t('applications.removeField')}
                          onClick={() => removeSignupField(row.name)}
                        >
                          <Trash2 className="size-4" />
                        </Button>
                      </div>
                    ))}
                    {form.signup_fields.length === 0 ? (
                      <p className="text-muted-foreground text-xs">{t('applications.signupFieldsEmpty')}</p>
                    ) : null}
                  </div>

                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button type="button" variant="outline" disabled={availableSignupFields.length === 0}>
                        <Plus className="size-4" />
                        {t('applications.addSignupField')}
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="start" className="w-56">
                      {availableSignupFields.map((option) => (
                        <DropdownMenuItem key={option.value} onSelect={() => addSignupField(option.value)}>
                          {option.label}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                  <p className="text-muted-foreground text-xs">{t('applications.signupFieldsHint')}</p>
                </div>
              </FormSection>

              <FormSection
                titleKey="applications.sectionSignupPage"
                descriptionKey="applications.sectionSignupPageHint"
              >
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field
                    id="app-signup-url"
                    labelKey="applications.signupUrl"
                    hint={t('applications.signupUrlHint')}
                  >
                    <Input
                      id="app-signup-url"
                      value={form.signup_url}
                      onChange={(event) => set('signup_url', event.target.value)}
                      placeholder={`/signup?client_id=…`}
                    />
                  </Field>
                  <Field
                    id="app-terms"
                    labelKey="applications.termsOfUse"
                    hint={t('applications.termsOfUseHint')}
                  >
                    <Input
                      id="app-terms"
                      value={form.terms_of_use}
                      onChange={(event) => set('terms_of_use', event.target.value)}
                      placeholder="https://…/terms"
                    />
                  </Field>
                  <Field
                    id="app-default-group"
                    labelKey="applications.defaultGroup"
                    hint={t('applications.defaultGroupHint')}
                  >
                    <Combobox
                      options={jobTitleOptions}
                      value={form.default_job_title_id}
                      onChange={(value) => set('default_job_title_id', value)}
                      placeholder={t('applications.notSet')}
                    />
                  </Field>
                </div>
              </FormSection>
            </>
          ) : null}
        </>
      ) : null}

      {current === 3 ? (
        <>
          <FormSection titleKey="applications.sectionClient" descriptionKey="applications.sectionClientHint">
            <div className="space-y-4">
              <div className="bg-muted/40 flex items-start gap-3 rounded-lg border px-3.5 py-3">
                <Badge variant="secondary">{t('applications.credentialsAuto')}</Badge>
                <p className="text-muted-foreground text-xs leading-relaxed">
                  {editing ? t('applications.credentialsAutoEdit') : t('applications.credentialsAutoHint')}
                </p>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field id="app-cert" labelKey="applications.cert" hint={t('applications.certHint')}>
                  <Combobox
                    options={certOptions}
                    value={form.cert_id}
                    onChange={(value) => set('cert_id', value)}
                    placeholder={t('applications.notSet')}
                  />
                </Field>
                <Field id="app-grants" labelKey="applications.grantTypes">
                  <MultiSelect
                    options={grantTypeOptions}
                    value={form.grant_types}
                    onChange={(next) => set('grant_types', next)}
                  />
                </Field>
              </div>
              <Field
                id="app-redirects"
                labelKey="applications.redirectUris"
                hint={t('applications.redirectHint')}
              >
                <RepeatableList
                  value={form.redirect_uris.length > 0 ? form.redirect_uris : ['']}
                  onChange={(next) => set('redirect_uris', next)}
                  placeholder="https://app.example.com/oauth/callback"
                  validate={(value) =>
                    invalidRedirectURI(value) ? t('applications.redirectInvalid') : undefined
                  }
                />
              </Field>
            </div>
          </FormSection>
        </>
      ) : null}

      {current === 4 ? (
        <>
          <FormSection titleKey="applications.sectionToken" descriptionKey="applications.sectionTokenHint">
            <div className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-3">
                <Field id="app-expire" labelKey="applications.expireInHours">
                  <Input
                    id="app-expire"
                    type="number"
                    step="0.5"
                    min={0}
                    value={form.expire_in_hours}
                    onChange={(event) => set('expire_in_hours', Number(event.target.value))}
                  />
                </Field>
                <Field id="app-refresh" labelKey="applications.refreshExpireInHours">
                  <Input
                    id="app-refresh"
                    type="number"
                    step="0.5"
                    min={0}
                    value={form.refresh_expire_in_hours}
                    onChange={(event) => set('refresh_expire_in_hours', Number(event.target.value))}
                  />
                </Field>
                <Field id="app-cookie" labelKey="applications.cookieExpireInHours">
                  <Input
                    id="app-cookie"
                    type="number"
                    min={0}
                    value={form.cookie_expire_in_hours}
                    onChange={(event) => set('cookie_expire_in_hours', Number(event.target.value))}
                  />
                </Field>
              </div>
              <Field
                id="app-token-format"
                labelKey="applications.tokenFormat"
                hint={t('applications.tokenFormatHint')}
              >
                <Select value={form.token_format} onValueChange={(value) => set('token_format', value)}>
                  <SelectTrigger id="app-token-format">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="standard">{t('applications.tokenFormatStandard')}</SelectItem>
                    <SelectItem value="custom">{t('applications.tokenFormatCustom')}</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              {form.token_format === 'custom' ? (
                <Field
                  id="app-token-fields"
                  labelKey="applications.tokenFields"
                  hint={t('applications.tokenFieldsHint')}
                >
                  <TokenFieldEditor
                    value={form.token_attributes}
                    onChange={(next) => set('token_attributes', next)}
                  />
                </Field>
              ) : null}
              <SwitchField
                labelKey="applications.disableRefreshRotation"
                descriptionKey="applications.desc.disableRefreshRotation"
                checked={form.disable_refresh_rotation}
                onCheckedChange={(checked) => set('disable_refresh_rotation', checked)}
              />
            </div>
          </FormSection>

          <FormSection titleKey="applications.sectionScopes" descriptionKey="applications.sectionScopesHint">
            <div className="space-y-3">
              <div className="text-muted-foreground hidden gap-2 px-3 text-xs sm:grid sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.4fr)_auto]">
                <span>{t('applications.scopeName')}</span>
                <span>{t('applications.scopeDisplayName')}</span>
                <span>{t('applications.scopeDescription')}</span>
                <span className="w-[104px]" />
              </div>
              <div className="space-y-2">
                {form.scopes.map((row, index) => (
                  <div
                    key={index}
                    className="grid items-center gap-2 rounded-lg border px-3 py-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.4fr)_auto]"
                  >
                    <Combobox
                      options={scopeOptions}
                      value={row.name}
                      allowCustom
                      clearable={false}
                      onChange={(value) => updateScope(index, { name: value })}
                      placeholder={t('applications.scopeNamePlaceholder')}
                    />
                    <Input
                      value={row.displayName}
                      onChange={(event) => updateScope(index, { displayName: event.target.value })}
                      placeholder={t('applications.scopeDisplayNamePlaceholder')}
                    />
                    <Input
                      value={row.description}
                      onChange={(event) => updateScope(index, { description: event.target.value })}
                      placeholder={t('applications.scopeDescriptionPlaceholder')}
                    />
                    <div className="flex items-center gap-0.5">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        disabled={index === 0}
                        aria-label={t('applications.moveUp')}
                        onClick={() => moveScope(index, -1)}
                      >
                        <ChevronUp className="size-4" />
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        disabled={index === form.scopes.length - 1}
                        aria-label={t('applications.moveDown')}
                        onClick={() => moveScope(index, 1)}
                      >
                        <ChevronDown className="size-4" />
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        aria-label={t('applications.removeScope')}
                        onClick={() => removeScope(index)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </div>
                ))}
                {form.scopes.length === 0 ? (
                  <p className="text-muted-foreground text-xs">{t('applications.scopesEmpty')}</p>
                ) : null}
              </div>
              <Button type="button" variant="outline" size="sm" onClick={addScope}>
                <Plus className="size-4" />
                {t('applications.addScope')}
              </Button>
            </div>
          </FormSection>

          <FormSection
            titleKey="applications.sectionSigninGuard"
            descriptionKey="applications.sectionSigninGuardHint"
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <Field
                id="app-signin-limit"
                labelKey="applications.signinErrorLimit"
                hint={t('applications.signinErrorLimitHint')}
              >
                <Input
                  id="app-signin-limit"
                  type="number"
                  min={0}
                  value={form.signin_error_limit}
                  onChange={(event) => set('signin_error_limit', Number(event.target.value))}
                />
              </Field>
              <Field
                id="app-signin-wait"
                labelKey="applications.signinRetryWaitMinutes"
                hint={t('applications.signinRetryWaitHint')}
              >
                <Input
                  id="app-signin-wait"
                  type="number"
                  min={0}
                  value={form.signin_retry_wait_minutes}
                  onChange={(event) => set('signin_retry_wait_minutes', Number(event.target.value))}
                />
              </Field>
            </div>
          </FormSection>

          <FormSection
            titleKey="applications.sectionAccess"
            descriptionKey="applications.sectionAccessHint"
            collapsible
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <Field
                id="app-ip-whitelist"
                labelKey="applications.ipWhitelist"
                hint={t('applications.ipHint')}
              >
                <Input
                  id="app-ip-whitelist"
                  value={form.ip_whitelist}
                  onChange={(event) => set('ip_whitelist', event.target.value)}
                  placeholder="10.0.0.0/8, 203.0.113.7"
                />
              </Field>
              <Field
                id="app-ip-restriction"
                labelKey="applications.ipRestriction"
                hint={t('applications.ipHint')}
              >
                <Input
                  id="app-ip-restriction"
                  value={form.ip_restriction}
                  onChange={(event) => set('ip_restriction', event.target.value)}
                />
              </Field>
              <Field
                id="app-org-choice"
                labelKey="applications.orgChoiceMode"
                hint={t('applications.orgChoiceModeHint')}
              >
                <Select
                  value={form.org_choice_mode || 'none'}
                  onValueChange={(value) => set('org_choice_mode', value === 'none' ? '' : value)}
                >
                  <SelectTrigger id="app-org-choice">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">none</SelectItem>
                    <SelectItem value="Select">Select</SelectItem>
                    <SelectItem value="Input">Input</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field id="app-forced-origin" labelKey="applications.forcedRedirectOrigin">
                <Input
                  id="app-forced-origin"
                  value={form.forced_redirect_origin}
                  onChange={(event) => set('forced_redirect_origin', event.target.value)}
                  placeholder="https://app.example.com"
                />
              </Field>
              <Field id="app-backchannel" labelKey="applications.backchannelLogoutUri">
                <Input
                  id="app-backchannel"
                  value={form.backchannel_logout_uri}
                  onChange={(event) => set('backchannel_logout_uri', event.target.value)}
                />
              </Field>
            </div>
          </FormSection>

          {form.token_format === 'custom' ? (
            <FormSection
              titleKey="applications.sectionAdapter"
              descriptionKey="applications.sectionAdapterHint"
              collapsible
            >
              <Field
                id="app-claim-map"
                labelKey="applications.claimMap"
                hint={t('applications.claimMapHint')}
              >
                <Textarea
                  id="app-claim-map"
                  rows={4}
                  value={form.claim_map}
                  onChange={(event) => set('claim_map', event.target.value)}
                  placeholder={'email=email\ndepartment=title\nemployeeNo=uid'}
                />
              </Field>
            </FormSection>
          ) : null}
        </>
      ) : null}

      {current === 5 ? (
        <FormSection titleKey="applications.sectionAccess" descriptionKey="applications.sectionAccessHint">
          <ApplicationAccessEditor access={access} onChange={onAccessChange} detail={accessDetail} />
        </FormSection>
      ) : null}
    </FormWizard>
  )
}
