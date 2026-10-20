import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from '@/shared/ui/toast'
import { Button, Input, Label, Spinner } from '@/shared/ui'
import { describeError } from '@/shared/lib/errors'
import type { AppInfo } from '@/shared/api/types'
import { authApi } from './auth-api'
import { AuthShell } from './auth-shell'
import { SubmitButton } from './components/submit-button'
import { useAuth } from './auth-context'

/** 注册字段名 → 表单输入的映射（未知字段落到 fields 透传） */
const FIELD_IDS: Record<string, string> = {
  Username: 'signup-username',
  'Display name': 'signup-display',
  'First name': 'signup-first',
  'Last name': 'signup-last',
  Email: 'signup-email',
  Phone: 'signup-phone',
  Password: 'signup-password',
  'Confirm password': 'signup-confirm',
  'Terms of use': 'signup-terms',
}

/**
 * 托管注册页（/signup）：由应用的 signup_items 编排驱动字段与必填校验。
 *
 * 与登录页一致地按 client_id 品牌化；应用未开启 self_signup_enabled 时直接展示"注册未开放"。
 */
export default function SignupPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { refresh, authenticated } = useAuth()
  const clientId = searchParams.get('client_id') ?? ''
  const institutionParam = searchParams.get('institution') ?? ''
  const applicationParam = searchParams.get('application') ?? ''

  const [appInfo, setAppInfo] = useState<AppInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [values, setValues] = useState<Record<string, string>>({})
  const [invalidFields, setInvalidFields] = useState<Record<string, boolean>>({})

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const info = clientId
          ? await authApi.appInfoByClientId(clientId)
          : await authApi.appInfo(undefined, institutionParam || undefined, applicationParam || undefined)
        if (!cancelled) setAppInfo(info)
      } catch {
        if (!cancelled) setAppInfo(null)
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [clientId, institutionParam, applicationParam])

  // 已登录用户不必停留在注册页
  useEffect(() => {
    if (authenticated) navigate('/portal', { replace: true })
  }, [authenticated, navigate])

  const items = useMemo(() => (appInfo?.signup_items ?? []).filter((item) => item.visible), [appInfo])

  const setValue = (name: string, value: string) => {
    setValues((current) => ({ ...current, [name]: value }))
    // 开始修正就撤掉红框，避免"边填边红"
    setInvalidFields((current) => (current[name] ? { ...current, [name]: false } : current))
  }

  const submit = useCallback(
    async (event: React.FormEvent) => {
      event.preventDefault()
      const missing = items.find((item) => item.required && !(values[item.name] ?? '').trim())
      if (missing) {
        setInvalidFields({ [missing.name]: true })
        document.getElementById(FIELD_IDS[missing.name] ?? `signup-${missing.name}`)?.focus()
        return
      }
      if (
        values['Password'] &&
        values['Confirm password'] &&
        values.Password !== values['Confirm password']
      ) {
        setInvalidFields({ Password: true, 'Confirm password': true })
        toast.error(t('signup.passwordMismatch'))
        return
      }
      setSubmitting(true)
      try {
        const extras: Record<string, string> = {}
        for (const item of items) {
          if (!['Username', 'Display name', 'Email', 'Phone', 'Password'].includes(item.name)) {
            extras[item.name] = values[item.name] ?? ''
          }
        }
        const result = await authApi.signup({
          // 有主键只传主键；仅当 app-info 尚未解析出主键时，才退回 URL 里的组织标识
          ...(appInfo?.institution_id
            ? { institution_id: appInfo.institution_id }
            : { institution: institutionParam || undefined }),
          application: appInfo?.code,
          client_id: clientId || undefined,
          name: values.Username ?? '',
          display_name: values['Display name'] ?? '',
          email: values.Email ?? '',
          phone: values.Phone ?? '',
          password: values.Password ?? '',
          fields: extras,
        })
        // signup_auto_signin 生效时后端已写入会话 Cookie
        if (result?.user) {
          await refresh()
          toast.success(t('signup.done'))
          navigate('/portal', { replace: true })
          return
        }
        toast.success(t('signup.done'))
        navigate('/login', { replace: true })
      } catch (error) {
        toast.error(describeError(error, t))
      } finally {
        setSubmitting(false)
      }
    },
    [items, values, appInfo, clientId, institutionParam, refresh, navigate, t],
  )

  const heading = appInfo?.name?.trim() || appInfo?.title?.trim() || t('signup.title')

  return (
    <AuthShell brandName={appInfo?.org_display_name} brandLogo={appInfo?.org_logo || appInfo?.logo}>
      <div className="flex flex-col gap-6">
        <header className="space-y-2 text-center">
          {appInfo?.logo ? (
            <img
              src={appInfo.logo}
              alt={appInfo.name || appInfo.code}
              className="mx-auto mb-3 size-11 rounded-xl object-contain"
            />
          ) : null}
          <h1 className="text-[26px] font-semibold tracking-tight">{heading}</h1>
          <p className="text-muted-foreground text-sm">{t('signup.subtitle')}</p>
        </header>

        {loading ? (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        ) : !appInfo?.self_signup_enabled ? (
          <div className="border-border rounded-md border border-dashed px-4 py-6 text-center">
            <p className="text-sm font-medium">{t('signup.closedTitle')}</p>
            <p className="text-muted-foreground mt-1 text-xs">{t('signup.closedHint')}</p>
            <Button variant="outline" className="mt-4" onClick={() => navigate('/login')}>
              {t('signup.backToLogin')}
            </Button>
          </div>
        ) : (
          <form className="space-y-4" noValidate onSubmit={submit}>
            {items.map((item) => {
              const inputId = FIELD_IDS[item.name] ?? `signup-${item.name}`
              const invalid = Boolean(invalidFields[item.name])
              return (
                <div key={item.name} className="space-y-2">
                  <Label htmlFor={inputId}>
                    {item.name}
                    {item.required ? <span className="text-destructive ml-1">*</span> : null}
                  </Label>
                  <Input
                    id={inputId}
                    type={
                      item.name === 'Password' || item.name === 'Confirm password'
                        ? 'password'
                        : item.name === 'Email'
                          ? 'email'
                          : 'text'
                    }
                    value={values[item.name] ?? ''}
                    aria-invalid={invalid || undefined}
                    aria-describedby={invalid ? `${inputId}-error` : undefined}
                    onChange={(event) => setValue(item.name, event.target.value)}
                    autoComplete={item.name === 'Password' ? 'new-password' : undefined}
                    className="h-10"
                    required={item.required}
                  />
                  {invalid ? (
                    <p id={`${inputId}-error`} role="alert" className="text-destructive text-xs">
                      {t('signup.fieldRequired', { field: item.name })}
                    </p>
                  ) : null}
                </div>
              )
            })}
            <SubmitButton pending={submitting}>{t('auth.signUp')}</SubmitButton>
            {appInfo?.terms_of_use?.trim() ? (
              <p className="text-muted-foreground text-center text-xs">
                {t('signup.termsPrefix')}{' '}
                <a
                  href={appInfo.terms_of_use}
                  target="_blank"
                  rel="noreferrer"
                  className="underline underline-offset-2"
                >
                  {t('signup.termsLink')}
                </a>
              </p>
            ) : null}
            <p className="text-muted-foreground text-center text-sm">
              {t('signup.haveAccount')}{' '}
              <Button type="button" variant="link" className="h-auto p-0" onClick={() => navigate('/login')}>
                {t('auth.signIn')}
              </Button>
            </p>
          </form>
        )}
      </div>
    </AuthShell>
  )
}
