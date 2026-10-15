import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { RefreshCw } from 'lucide-react'
import {
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/shared/ui'
import { ApiError } from '@/shared/api/client'
import { describeError } from '@/shared/lib/errors'
import type { AppInfo, CaptchaChallenge, ProviderInfo } from '@/shared/api/types'
import { authApi } from './auth-api'
import { SigninMethodIcon } from '@/shared/components/signin-method-icon'
import { ProviderIcon } from '@/shared/components/provider-icon'
import i18n from '@/shared/i18n'
import { AuthShell } from './auth-shell'
import { MfaVerifyStep } from './components/mfa-verify-step'
import { MfaEnrollStep } from './components/mfa-enroll-step'
import { useAuth } from './auth-context'

/** 第三方登录按钮的可访问名称：组织命名优先（可区分同类多个），否则用类型词条 */
function providerLabel(provider: ProviderInfo, t: (key: string) => string): string {
  const named = provider.display_name?.trim()
  if (named) return named
  const type = provider.type ?? ''
  const key = `auth.providerName.${type}`
  return type && i18n.exists(key) ? t(key) : provider.name
}

/** 默认页签：优先密码，其次验证码，最后魔法链接（与登录页展示顺序一致） */
function defaultMode(methods: string[]): 'password' | 'code' | 'magic' {
  if (methods.length === 0 || methods.includes('Password')) return 'password'
  if (methods.includes('EmailCode') || methods.includes('SMSCode')) return 'code'
  return 'magic'
}

/**
 * 登录页：由后端 /auth/app-info 驱动（登录方式、字段编排、人机校验、可用提供方）。
 * 支持 密码登录、邮箱/短信验证码登录、第三方登录三类入口。
 *
 * 视觉为极简取向（对齐 folk）：单列居中，墨黑主按钮，第三方登录收在表单下方的次选位。
 *
 * 应用驱动登录（对齐 Casdoor /login/oauth/authorize）：当 URL 携带 OAuth 参数
 * （client_id/redirect_uri/…）时，按该 client_id 对应的应用渲染品牌，登录成功后
 * 续走 /oauth/authorize 并回跳应用的 redirect_uri。
 */
export default function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { refresh, authenticated, isAdmin } = useAuth()
  // 登录页使用公开端点获取组织（/auth/institutions），不请求受保护的组织接口
  const [institutions, setInstitutions] = useState<
    { id: number; name: string; display_name: string }[]
  >([])
  // 组织由后端自动解析（域名 / 唯一账号命中 / 默认组织），登录页**默认为空**；
  // 仅当应用驱动（app-info 携带组织）或用户显式选择时才有值——它是一个"覆盖项"，不是必需项。
  // 注意：不读 localStorage['polaris.institution']，避免把控制台的"上次浏览组织"误带进登录。
  const [institution, setInstitution] = useState('')
  const [institutionRequired, setInstitutionRequired] = useState(false)

  // OAuth 应用驱动参数：带 client_id 即按该应用品牌化（品牌、提供方、组织都按应用解析）
  const oauthQuery = useMemo(() => {
    const keys = [
      'client_id',
      'redirect_uri',
      'response_type',
      'scope',
      'state',
      'nonce',
      'code_challenge',
      'code_challenge_method',
    ] as const
    const params = new URLSearchParams()
    for (const key of keys) {
      const value = searchParams.get(key)
      if (value != null) params.set(key, value)
    }
    return params
  }, [searchParams])
  const clientId = searchParams.get('client_id') ?? ''
  const redirectUri = searchParams.get('redirect_uri') ?? ''
  // 品牌化只看 client_id：不带 client_id 时才回退到"组织默认应用"。
  // 之前要求 client_id + redirect_uri 同时存在才品牌化，导致"应用没配回调地址"时
  // 直接掉进默认应用（内置应用）——这正是"新建应用后访问授权端点却是内置应用"的根因。
  const appDriven = Boolean(clientId)
  // 登录后能否续走 /oauth/authorize 并回跳应用：OAuth 回跳必须带 redirect_uri，缺一不可
  const canResumeAuthorize = Boolean(clientId && redirectUri)

  const from = useMemo(() => {
    const raw = searchParams.get('from') ?? ''
    return raw.startsWith('/') && !raw.startsWith('//') && !raw.startsWith('/login') ? raw : ''
  }, [searchParams])

  const [providers, setProviders] = useState<ProviderInfo[]>([])

  // institutionID：组织主键，公开面的**权威引用**（institution 仅为兼容/展示用的组织标识）
  const [institutionID, setInstitutionID] = useState<number | null>(null)
  // 公开面请求的组织参数（唯一出口）：**有主键就只传 institution_id**；
  // 仅当"组织下拉无候选、用户手输组织标识"这一旧路径上才退回 institution。
  // 两个字段不同时传——同请求里两个来源会产生"谁是权威"的歧义。
  const orgRef = useMemo<{ institution_id?: number; institution?: string }>(() => {
    if (institutionID) return { institution_id: institutionID }
    return institution ? { institution } : {}
  }, [institutionID, institution])
  const [appInfo, setAppInfo] = useState<AppInfo | null>(null)
  const [captcha, setCaptcha] = useState<CaptchaChallenge>({ required: false })
  const [mode, setMode] = useState<'password' | 'code' | 'magic'>('password')

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  // 登录流程分屏：凭据 → （组织要求时）第二因子 / 绑定。
  // 对齐成熟产品（Casdoor `MfaVerify`）：第二因子是**独立一屏**，
  // 而不是在凭据表单里再塞一个「动态码」字段、把账号密码再提交一遍。
  const [step, setStep] = useState<'credentials' | 'mfa' | 'enroll'>('credentials')
  const [mfaRememberHours, setMfaRememberHours] = useState(0)
  const [enrollSecret, setEnrollSecret] = useState('')
  const [enrollUrl, setEnrollUrl] = useState('')
  const [captchaCode, setCaptchaCode] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const [magicEmail, setMagicEmail] = useState('')
  const [magicSent, setMagicSent] = useState(false)
  const [magicSending, setMagicSending] = useState(false)

  const [codeKind, setCodeKind] = useState<'email' | 'sms'>('email')
  const [codeTarget, setCodeTarget] = useState('')
  const [code, setCode] = useState('')
  const [sending, setSending] = useState(false)
  const [countdown, setCountdown] = useState(0)
  const timerRef = useRef<number | null>(null)

  // 引导请求（status/institutions/providers/app-info）必须**每个 key 只发一次**：
  // 之前 loadAppInfo 会把解析出的组织写回 state，而它又依赖这两个 state → 依赖变化 → 再请求一轮，
  // 于是打开登录页能看到 providers/app-info 各 3 次、status 4 次（StrictMode 再叠加一倍）。
  const bootstrapKeyRef = useRef('')
  const institutionsLoadedRef = useRef(false)
  // 供 loader 读取"最新组织"而不把它放进依赖（避免自触发）
  const institutionRef = useRef(institution)
  const institutionIDRef = useRef(institutionID)
  useEffect(() => {
    institutionRef.current = institution
    institutionIDRef.current = institutionID
  }, [institution, institutionID])

  const loadInstitutions = useCallback(async () => {
    if (institutionsLoadedRef.current) return
    institutionsLoadedRef.current = true
    try {
      const result = await authApi.institutions()
      // 仅用于"需要用户显式选择"时的下拉候选；不在此时预选，交给后端解析
      setInstitutions(result.institutions ?? [])
    } catch {
      setInstitutions([])
    }
  }, [])

  const loadProviders = useCallback(async () => {
    try {
      // 应用驱动登录：按 client_id 定位应用；否则按组织解析
      const result = appDriven
        ? await authApi.providers(undefined, undefined, undefined, clientId)
        : await authApi.providers(institutionIDRef.current ?? undefined, undefined)
      setProviders(result.providers ?? [])
    } catch {
      setProviders([])
    }
  }, [appDriven, clientId])

  const loadAppInfo = useCallback(async () => {
    // 组织可留空：后端按域名 / 唯一租户 / 默认租户自动解析
    try {
      const info = appDriven
        ? await authApi.appInfoByClientId(clientId)
        : await authApi.appInfo(institutionIDRef.current ?? undefined, undefined)
      setAppInfo(info)
      // 应用驱动：以 app-info 的组织主键锁定登录组织（权威引用）；
      // 组织标识仅用于选择器展示与兼容请求字段。
      if (appDriven) {
        if (info.institution_id) setInstitutionID(info.institution_id)
        if (info.institution) setInstitution(info.institution)
      }
      // 仅"应用驱动登录"时按应用锁定组织；普通登录不采用 app-info 的组织——
      // app-info 未指定组织时解析到的是**默认组织**，若据此固定登录组织，
      // 会让"账号唯一命中优先于默认组织"的解析策略失效（多租户下非默认组织用户被截胡）。
      // 第三方入口与「登录方式」同一套判定（与后端 availableSigninMethods 同源）：
      // 应用没把 Provider 列为登录方式，或没配置任何提供方 → **一个第三方入口都不显示**。
      // 事故背景：读取侧曾把"providers 为空"当成"全都要"，于是没配钉钉的应用登录页出现钉钉按钮；
      // 这里保留前端兜底，避免"接口宽松 + 页面宽松"叠成假入口。
      const providerSigninEnabled = (info.signin_methods ?? []).some((item) => item.name === 'Provider')
      if (!providerSigninEnabled) {
        setProviders([])
      } else if (info.providers?.length) {
        const enabled = new Set(info.providers.filter((item) => item.canSignIn).map((item) => item.name))
        setProviders((current) => current.filter((provider) => enabled.has(provider.name)))
      }
      if (info.captcha_required) {
        const challenge = await authApi.captcha(info.institution_id || institutionID || undefined)
        setCaptcha(challenge)
      } else {
        setCaptcha({ required: false })
      }
      setMode(defaultMode(info.signin_methods?.map((item) => item.name) ?? []))
    } catch {
      setAppInfo(null)
    }
  }, [appDriven, clientId])

  useEffect(() => {
    void loadInstitutions()
  }, [loadInstitutions])

  // 已登录用户访问登录页：应用驱动则续走授权，否则按角色落地（对齐 Casdoor）
  const autoRedirected = useRef(false)
  useEffect(() => {
    if (!authenticated || autoRedirected.current) return
    autoRedirected.current = true
    if (canResumeAuthorize) {
      window.location.assign(`/oauth/authorize?${oauthQuery.toString()}`)
      return
    }
    if (from) {
      navigate(from, { replace: true })
      return
    }
    navigate(isAdmin ? '/' : '/portal', { replace: true })
  }, [authenticated, isAdmin, canResumeAuthorize, oauthQuery, from, navigate])

  useEffect(() => {
    // 同一个"组织/应用"只加载一次：组织被 app-info 写入、或用户在组织选择器上改选时，
    // 只有 key 真正变化才重新请求（改选组织是显式动作，应当重新加载）。
    const key = appDriven ? `client:${clientId}` : `org:${institutionID ?? ''}:${institution ?? ''}`
    if (bootstrapKeyRef.current === key) return
    bootstrapKeyRef.current = key
    void loadProviders()
    void loadAppInfo()
  }, [appDriven, clientId, institution, institutionID, loadProviders, loadAppInfo])

  useEffect(() => {
    return () => {
      if (timerRef.current) window.clearInterval(timerRef.current)
    }
  }, [])

  const refreshCaptcha = useCallback(async () => {
    try {
      setCaptcha(await authApi.captcha(institutionID ?? undefined))
      setCaptchaCode('')
    } catch {
      setCaptcha({ required: false })
    }
  }, [institutionID])

  const finishLogin = useCallback(
    async (resolvedInstitution?: string, target?: string) => {
      // 以**后端解析出的组织**为准写入控制台偏好，使其落地到正确组织（而非登录页的猜测）
      const landingOrg = resolvedInstitution || institution
      if (landingOrg) window.localStorage.setItem('polaris.institution', landingOrg)
      await refresh()
      // 应用驱动登录：登录成功后回到 /oauth/authorize 续走授权，由其按 redirect_uri 回跳应用
      // （缺 redirect_uri 时无法回跳，退化为按角色落地——应用侧应补齐回调地址）
      if (canResumeAuthorize) {
        window.location.assign(`/oauth/authorize?${oauthQuery.toString()}`)
        return
      }
      // 显式 from 优先；否则按角色落地——管理员进控制台，普通用户进门户
      if (target && target.startsWith('/') && !target.startsWith('//')) {
        navigate(target, { replace: true })
        return
      }
      if (from) {
        navigate(from, { replace: true })
        return
      }
      const status = await authApi.status().catch(() => null)
      navigate(status?.is_admin ? '/' : '/portal', { replace: true })
    },
    [refresh, navigate, from, institution, canResumeAuthorize, oauthQuery],
  )

  // 完成 MFA 绑定：校验动态码 → 解除会话受限 → 完成登录
  const submitMfaEnroll = useCallback(
    async (code: string) => {
      setSubmitting(true)
      try {
        await authApi.mfaVerify(code)
        toast.success(t('auth.mfaEnrollDone'))
        await finishLogin()
      } catch (error) {
        toast.error(describeError(error, t))
      } finally {
        setSubmitting(false)
      }
    },
    [finishLogin, t],
  )

  // 第二因子提交：带上已保存的凭据 + 动态码（恢复码走同一字段，后端两者都认）
  const submitMfaCode = useCallback(
    async (code: string, remember: boolean) => {
      setSubmitting(true)
      try {
        const result = await authApi.login({
          ...orgRef,
          // 应用上下文：应用驱动登录页（?client_id=）必须把 client_id 带上，
          // 否则后端按"组织默认应用"解析，应用的准入判定与登录方式都会落空。
          client_id: clientId || undefined,
          username,
          password,
          mfa_code: code,
          remember_mfa: remember,
          from,
          captcha_id: captcha.type === 'image' ? captcha.captcha_id : undefined,
          captcha_code: captcha.type === 'image' ? captchaCode : undefined,
          captcha_token: captcha.type !== 'image' ? captchaCode : undefined,
        })
        await finishLogin(String(result.user?.institution_id ?? ''), result.session?.from)
      } catch (error) {
        toast.error(describeError(error, t))
        await refreshCaptcha()
      } finally {
        setSubmitting(false)
      }
    },
    [orgRef, username, password, from, captcha, captchaCode, refreshCaptcha, finishLogin, t],
  )

  // 返回凭据屏：清掉第二因子中间态，并换一份新的人机校验挑战
  const backToCredentials = useCallback(async () => {
    setStep('credentials')
    setMfaRememberHours(0)
    await refreshCaptcha()
  }, [refreshCaptcha])

  const submitMagicLink = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!magicEmail.trim()) return
    setMagicSending(true)
    try {
      await authApi.sendMagicLink({
        ...orgRef,
        client_id: clientId || undefined,
        email: magicEmail.trim(),
        from,
        captcha_id: captcha.type === 'image' ? captcha.captcha_id : undefined,
        captcha_code: captcha.type === 'image' ? captchaCode : undefined,
        captcha_token: captcha.type !== 'image' ? captchaCode : undefined,
      })
      // 无论邮箱是否存在都提示"已发送"：后端刻意不泄露账号存在性
      setMagicSent(true)
    } catch (error) {
      toast.error(describeError(error, t))
    } finally {
      setMagicSending(false)
    }
  }

  const submitPassword = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!username || !password) return
    if (captcha.required && captcha.type === 'image' && !captchaCode) {
      toast.error(t('auth.captchaRequired'))
      return
    }
    setSubmitting(true)
    try {
      const result = await authApi.login({
        ...orgRef,
        // 应用上下文：应用驱动登录页（?client_id=）必须把 client_id 带上，
        // 否则后端按"组织默认应用"解析，应用的准入判定与登录方式都会落空。
        client_id: clientId || undefined,
        username,
        password,
        from,
        captcha_id: captcha.type === 'image' ? captcha.captcha_id : undefined,
        captcha_code: captcha.type === 'image' ? captchaCode : undefined,
        captcha_token: captcha.type !== 'image' ? captchaCode : undefined,
      })
      if (result.mfa?.enroll) {
        // 组织强制二次验证且尚未绑定：会话已建立但受限 → 切到「绑定」屏
        try {
          const setup = await authApi.mfaSetup()
          setEnrollSecret(setup.secret)
          setEnrollUrl(setup.url)
          setStep('enroll')
        } catch (error) {
          toast.error(describeError(error, t))
        }
        return
      }
      if (result.mfa?.required) {
        // 已绑定的用户：切到独立的第二因子屏（凭据保存在 state，提交动态码时一并带上）
        setMfaRememberHours(result.mfa.remember_hours ?? 0)
        setStep('mfa')
        // 人机校验一次性消费：第二次提交需要新的挑战
        await refreshCaptcha()
        return
      }
      await finishLogin(String(result.user?.institution_id ?? ''))
    } catch (error) {
      // 后端无法唯一解析组织（同名账号存在于多个组织）时，才要求用户显式选择
      if (error instanceof ApiError && error.code === 'INSTITUTION_REQUIRED') {
        setInstitutionRequired(true)
      }
      toast.error(describeError(error, t))
      await refreshCaptcha()
    } finally {
      setSubmitting(false)
    }
  }

  const sendCode = async () => {
    if (!codeTarget) return
    setSending(true)
    try {
      await authApi.sendCode({
        institution,
        client_id: clientId || undefined,
        kind: codeKind,
        target: codeTarget,
        captcha_id: captcha.type === 'image' ? captcha.captcha_id : undefined,
        captcha_code: captcha.type === 'image' ? captchaCode : undefined,
        captcha_token: captcha.type !== 'image' ? captchaCode : undefined,
      })
      toast.success(t('auth.codeSent'))
      setCountdown(60)
      timerRef.current = window.setInterval(() => {
        setCountdown((value) => {
          if (value <= 1 && timerRef.current) window.clearInterval(timerRef.current)
          return value <= 1 ? 0 : value - 1
        })
      }, 1000)
      await refreshCaptcha()
    } catch (error) {
      toast.error(describeError(error, t))
      await refreshCaptcha()
    } finally {
      setSending(false)
    }
  }

  const submitCode = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!codeTarget || !code) return
    setSubmitting(true)
    try {
      const result = await authApi.codeLogin({
        institution,
        client_id: clientId || undefined,
        kind: codeKind,
        target: codeTarget,
        code,
        from,
      })
      await finishLogin(String(result.user?.institution_id ?? ''), result.session?.from)
    } catch (error) {
      toast.error(describeError(error, t))
      await refreshCaptcha()
    } finally {
      setSubmitting(false)
    }
  }

  const startSso = async (provider: string) => {
    try {
      const result = await authApi.oauthAuthorize(provider, from, institutionID ?? undefined, clientId || undefined)
      window.location.assign(result.authorization_url)
    } catch (error) {
      toast.error(describeError(error, t))
    }
  }

  const captchaBlock = captcha.required ? (
    <div className="space-y-2">
      <Label htmlFor="captcha-code">{t('auth.captcha')}</Label>
      <div className="flex items-center gap-2">
        <Input
          id="captcha-code"
          value={captchaCode}
          onChange={(event) => setCaptchaCode(event.target.value)}
          placeholder={t('auth.captchaPlaceholder')}
          autoComplete="off"
          className="h-10"
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-muted-foreground size-10 shrink-0"
          onClick={() => void refreshCaptcha()}
          aria-label={t('common.refresh')}
        >
          <RefreshCw className="size-4" />
        </Button>
      </div>
      {captcha.type === 'image' && captcha.image ? (
        <img
          src={captcha.image}
          alt={t('auth.captcha')}
          className="h-12 cursor-pointer rounded-md border"
          onClick={() => void refreshCaptcha()}
        />
      ) : (
        <p className="text-muted-foreground text-xs">
          {t('auth.captchaRemoteHint', { type: captcha.type, siteKey: captcha.site_key ?? '-' })}
        </p>
      )}
    </div>
  ) : null

  // 组织选择器（对齐 Casdoor）：默认不展示——组织由域名/应用/后端自动解析；
  // 仅当应用显式设置 org_choice_mode（Select/Input），或后端判定账号歧义时才展示。
  const orgChoiceMode = appInfo?.org_choice_mode ?? ''
  const showInstitutionSelector =
    institutionRequired || orgChoiceMode === 'Select' || orgChoiceMode === 'Input'
  // 登录方式完全由后端下发的 signin_methods 驱动（后端已按组织能力过滤）
  const methodNames = (appInfo?.signin_methods ?? []).map((item) => item.name)
  // app-info 尚未返回时保守放行密码，避免首帧闪现"无可用登录方式"
  const passwordAvailable = appInfo == null || methodNames.includes('Password')
  const codeAvailable = methodNames.includes('EmailCode') || methodNames.includes('SMSCode')
  const magicAvailable = methodNames.includes('MagicLink')
  const signupAvailable = Boolean(appInfo?.self_signup_enabled)
  const magicTtlMinutes = Math.max(1, Math.round((appInfo?.magic_link_ttl_seconds ?? 600) / 60))
  // 注册入口：应用可外链到自己的注册页（signup_url）；未配置则用平台托管的 /signup
  const configuredSignupUrl = appInfo?.signup_url?.trim() ?? ''
  const signupHref =
    configuredSignupUrl || (clientId ? `/signup?client_id=${encodeURIComponent(clientId)}` : '/signup')
  const signupIsExternal = configuredSignupUrl !== '' && !configuredSignupUrl.startsWith('/')
  const emailAvailable = (appInfo?.email_providers?.length ?? 0) > 0
  const smsAvailable = (appInfo?.sms_providers?.length ?? 0) > 0

  useEffect(() => {
    if (!emailAvailable && smsAvailable) setCodeKind('sms')
    if (emailAvailable && !smsAvailable) setCodeKind('email')
  }, [emailAvailable, smsAvailable])

  // 极简下划线式标签：去掉默认的胶囊底色，仅用文字与底线区分选中态
  const tabTriggerClass =
    'text-muted-foreground hover:text-foreground flex-none justify-start rounded-none border-b-2 border-transparent bg-transparent px-0 pt-1 pb-2.5 text-sm font-medium transition-colors data-[state=active]:border-foreground data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none'

  // 登录页品牌文案取值（与向导实时预览保持一致）：
  //   大标题 = 应用驱动的「显示名称」→「应用标题」→ 通用欢迎语；
  //   副标题 = 「应用标题」（不与大标题重复时）→ 登录引导语。
  const heading =
    (appDriven ? appInfo?.name?.trim() : '') || appInfo?.title?.trim() || t('auth.welcomeTitle')
  // 欢迎语取自**组织描述**（不再显示平台名）；组织未填描述时退回应用标题，仍为空则不显示。
  const orgDescription = appInfo?.org_description?.trim() || ''
  const appTitle = appInfo?.title?.trim() || ''
  const subtitle = orgDescription || (appTitle && appTitle !== heading ? appTitle : '')

  // 卡片图标：当前应用的 icon 优先，应用未配置时回退组织 Logo（避免登录框空一块）
  const cardLogo = appInfo?.logo || appInfo?.org_logo || ''
  // 浏览器页签图标：组织的 favicon（而非平台默认）；React 19 会把 <link> 提升到 <head>
  const orgFavicon = appInfo?.org_favicon || ''

  return (
    <>
      {orgFavicon ? <link rel="icon" href={orgFavicon} /> : null}
    <AuthShell brandName={appInfo?.org_display_name} brandLogo={appInfo?.org_logo || appInfo?.logo}>
      <div className="flex flex-col gap-6">
        <header className="space-y-2 text-center">
          {cardLogo ? (
            <img
              src={cardLogo}
              alt={appInfo?.name || appInfo?.title || t('app.name')}
              className="mx-auto mb-3 size-11 rounded-xl object-contain"
            />
          ) : null}
          <h1 className="text-[26px] font-semibold tracking-tight">{heading}</h1>
          {subtitle ? <p className="text-muted-foreground text-sm leading-relaxed">{subtitle}</p> : null}
        </header>

        {appDriven && !canResumeAuthorize ? (
          <p className="text-muted-foreground border-border rounded-md border border-dashed px-3 py-2 text-center text-xs leading-relaxed">
            {t('auth.appNoRedirectHint')}
          </p>
        ) : null}

        {showInstitutionSelector ? (
          <div className="space-y-2">
            <Label htmlFor="institution">{t('auth.institution')}</Label>
            {institutionRequired ? (
              <p className="text-muted-foreground text-xs">{t('auth.institutionAmbiguous')}</p>
            ) : null}
            {institutions.length > 0 ? (
              <Select
                value={institutionID ? String(institutionID) : ''}
                onValueChange={(value) => {
                  const picked = institutions.find((option) => String(option.id) === value)
                  setInstitutionID(picked ? picked.id : null)
                  setInstitution(picked?.name ?? '')
                }}
              >
                <SelectTrigger id="institution" className="h-10 w-full">
                  <SelectValue placeholder={t('auth.institutionPlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {institutions.map((option) => (
                    <SelectItem key={option.id} value={String(option.id)}>
                      {option.display_name || option.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                id="institution"
                value={institution}
                onChange={(event) => setInstitution(event.target.value)}
                placeholder="built-in"
                className="h-10"
              />
            )}
          </div>
        ) : null}

        <Tabs value={mode} onValueChange={(value) => setMode(value as 'password' | 'code')}>
          {/* 仅一种登录方式时不必渲染切换条，避免出现只有一个标签的空壳 */}
          {[passwordAvailable, codeAvailable, magicAvailable].filter(Boolean).length > 1 ? (
            <TabsList
              variant="plain"
              className="h-auto w-full justify-start gap-6 rounded-none border-b bg-transparent p-0"
            >
              {passwordAvailable ? (
                <TabsTrigger value="password" className={tabTriggerClass}>
                  <SigninMethodIcon name="Password" className="size-3.5" />
                  {t('auth.passwordTab')}
                </TabsTrigger>
              ) : null}
              {codeAvailable ? (
                <TabsTrigger value="code" className={tabTriggerClass}>
                  <SigninMethodIcon name={emailAvailable ? 'EmailCode' : 'SMSCode'} className="size-3.5" />
                  {t('auth.codeTab')}
                </TabsTrigger>
              ) : null}
              {magicAvailable ? (
                <TabsTrigger value="magic" className={tabTriggerClass}>
                  <SigninMethodIcon name="MagicLink" className="size-3.5" />
                  {t('auth.magicTab')}
                </TabsTrigger>
              ) : null}
            </TabsList>
          ) : null}

          <TabsContent value="password" className="pt-6">
            {step === 'mfa' ? (
              /* 第二因子：独立一屏；凭据已保存，只补动态码 */
              <MfaVerifyStep
                rememberHours={mfaRememberHours}
                pending={submitting}
                onSubmit={(code, remember) => void submitMfaCode(code, remember)}
                onBack={() => void backToCredentials()}
              >
                {captchaBlock}
              </MfaVerifyStep>
            ) : step === 'enroll' ? (
              /* 组织强制二次验证：独立绑定屏（扫码为主 + 手动兜底） */
              <MfaEnrollStep
                secret={enrollSecret}
                url={enrollUrl}
                pending={submitting}
                onSubmit={(code) => void submitMfaEnroll(code)}
                onBack={() => void backToCredentials()}
              />
            ) : (
              <form className="space-y-4" onSubmit={submitPassword}>
                <div className="space-y-2">
                  <Label htmlFor="username">{t('auth.username')}</Label>
                  <Input
                    id="username"
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
                    placeholder={t('auth.usernamePlaceholder')}
                    autoComplete="username"
                    className="h-10"
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="password">{t('auth.password')}</Label>
                  <Input
                    id="password"
                    type="password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    autoComplete="current-password"
                    className="h-10"
                    required
                  />
                </div>

                {captchaBlock}

                <Button type="submit" className="h-10 w-full" disabled={submitting}>
                  {submitting ? <Spinner /> : null}
                  {submitting ? t('auth.signingIn') : t('auth.signIn')}
                </Button>
                {showInstitutionSelector && !institution ? (
                  <p className="text-muted-foreground text-xs">{t('auth.institutionRequired')}</p>
                ) : null}
              </form>
            )}
          </TabsContent>

          {codeAvailable ? (
            <TabsContent value="code" className="pt-6">
              <form className="space-y-4" onSubmit={submitCode}>
                <div className="space-y-2">
                  <Label htmlFor="code-kind">{t('auth.codeKind')}</Label>
                  <Select value={codeKind} onValueChange={(value) => setCodeKind(value as 'email' | 'sms')}>
                    <SelectTrigger id="code-kind" className="h-10 w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {emailAvailable ? <SelectItem value="email">{t('auth.codeEmail')}</SelectItem> : null}
                      {smsAvailable ? <SelectItem value="sms">{t('auth.codeSms')}</SelectItem> : null}
                      {!emailAvailable && !smsAvailable ? (
                        <SelectItem value="email">{t('auth.codeEmail')}</SelectItem>
                      ) : null}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="code-target">{t('auth.codeTarget')}</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      id="code-target"
                      value={codeTarget}
                      onChange={(event) => setCodeTarget(event.target.value)}
                      placeholder={codeKind === 'email' ? 'you@example.com' : '13800000000'}
                      className="h-10"
                      required
                    />
                    <Button
                      type="button"
                      variant="outline"
                      className="h-10 shrink-0"
                      disabled={sending || countdown > 0 || !codeTarget}
                      onClick={() => void sendCode()}
                    >
                      {countdown > 0 ? `${countdown}s` : t('auth.sendCode')}
                    </Button>
                  </div>
                </div>
                {captchaBlock}
                <div className="space-y-2">
                  <Label htmlFor="code-value">{t('auth.codeValue')}</Label>
                  <Input
                    id="code-value"
                    value={code}
                    onChange={(event) => setCode(event.target.value)}
                    inputMode="numeric"
                    maxLength={8}
                    autoComplete="one-time-code"
                    className="h-10"
                    required
                  />
                </div>
                <Button type="submit" className="h-10 w-full" disabled={submitting}>
                  {submitting ? <Spinner /> : null}
                  {t('auth.codeSignIn')}
                </Button>
              </form>
            </TabsContent>
          ) : null}

          {magicAvailable ? (
            <TabsContent value="magic" className="pt-6">
              <form className="space-y-4" onSubmit={submitMagicLink}>
                <div className="space-y-2">
                  <Label htmlFor="magic-email">{t('auth.email')}</Label>
                  <Input
                    id="magic-email"
                    type="email"
                    value={magicEmail}
                    onChange={(event) => setMagicEmail(event.target.value)}
                    placeholder="you@example.com"
                    autoComplete="email"
                    className="h-10"
                    required
                  />
                  <p className="text-muted-foreground text-xs">
                    {t('auth.magicHint', { minutes: magicTtlMinutes })}
                  </p>
                </div>
                {captchaBlock}
                {magicSent ? <p className="text-muted-foreground text-sm">{t('auth.magicSent')}</p> : null}
                <Button type="submit" className="h-10 w-full" disabled={magicSending}>
                  {magicSending ? <Spinner /> : null}
                  {t('auth.magicSubmit')}
                </Button>
              </form>
            </TabsContent>
          ) : null}
        </Tabs>

        {/* 第三方登录入口只在**凭据屏**显示：进入第二因子 / 绑定屏后，当前流程是"补完这一因子"，
            再摆一排第三方位容易让人误以为可以绕开二次验证 */}
        {step === 'credentials' && providers.length > 0 ? (
          <div className="space-y-4">
            <div className="text-muted-foreground flex items-center gap-3 text-xs">
              <span className="bg-border h-px flex-1" />
              {t('auth.or')}
              <span className="bg-border h-px flex-1" />
            </div>
            {/* 第三方登录：品牌 logo 按钮（后端下发 type/icon，前端按 key 取素材） */}
            <div className="flex flex-wrap items-center justify-center gap-2.5">
              {providers.map((provider) => {
                const label = providerLabel(provider, t)
                return (
                  <Button
                    key={provider.name}
                    type="button"
                    variant="outline"
                    size="icon"
                    className="size-11"
                    title={label}
                    aria-label={label}
                    onClick={() => void startSso(provider.name)}
                  >
                    <ProviderIcon iconKey={provider.icon || provider.type || ''} size={24} />
                  </Button>
                )
              })}
            </div>
          </div>
        ) : null}

        {signupAvailable ? (
          <p className="text-muted-foreground text-center text-sm">
            {t('auth.noAccount')}{' '}
            {signupIsExternal ? (
              <Button asChild variant="link" className="h-auto p-0">
                <a href={signupHref} rel="noreferrer">
                  {t('auth.signUp')}
                </a>
              </Button>
            ) : (
              <Button type="button" variant="link" className="h-auto p-0" onClick={() => navigate(signupHref)}>
                {t('auth.signUp')}
              </Button>
            )}
          </p>
        ) : null}
      </div>
    </AuthShell>
    </>
  )
}
