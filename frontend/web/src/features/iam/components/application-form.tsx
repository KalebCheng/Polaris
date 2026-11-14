import type { SelectOption } from '@/shared/ui'
import type { Application, ApplicationAccess, TokenAttribute } from '@/shared/api/types'

/**
 * 可申请的 scope 行（一行一个 scope，顺序即授权时的推荐顺序）。
 * 名称支持"选择常用值或输入自定义值"（自定义 scope 可直接填名字）。
 */
export interface ScopeRow {
  name: string
  displayName: string
  description: string
}

/**
 * 注册表单字段行（**一行一个字段**，顺序即注册页展示顺序）。
 * Username / Password 是内置字段：注册接口必需，不可删除且必填锁定。
 */
export interface SignupFieldRow {
  name: string
  required: boolean
  /** 内置字段（用户名 / 密码） */
  builtin?: boolean
}

/** 注册接口必需的内置字段（不可删除、必填锁定） */
export const SIGNUP_BUILTIN_FIELDS = ['Username', 'Password'] as const

/** 应用表单状态（与后端字段一一对应） */
export interface ApplicationFormValue {
  institution_id: number
  /** 应用标识：进入 URL 与外部集成，创建后不可修改 */
  code: string
  /** 应用名称：人类可读 */
  name: string
  category: string
  type: string
  logo: string
  logo_dark: string
  favicon: string
  title: string
  description: string
  homepage_url: string
  /** 标签集合（chips 输入） */
  tags: string[]
  cert_id: string
  default_job_title_id: string
  org_choice_mode: string
  ip_restriction: string
  ip_whitelist: string
  /** 注册入口地址（留空则用平台托管注册页 /signup） */
  signup_url: string
  /** 用户协议链接（注册页渲染） */
  terms_of_use: string

  // 登录方式：只保存组织级方法主键，方法参数全部在组织配置页维护
  signin_method_ids: number[]

  // 自助注册
  self_signup_enabled: boolean
  signup_auto_signin: boolean
  signup_fields: SignupFieldRow[]

  // 会话策略
  max_sessions: number

  // 登录失败限制（0 = 沿用平台默认）
  signin_error_limit: number
  signin_retry_wait_minutes: number

  // 客户端与令牌
  grant_types: string[]
  redirect_uris: string[]
  scopes: ScopeRow[]
  expire_in_hours: number
  refresh_expire_in_hours: number
  cookie_expire_in_hours: number
  disable_refresh_rotation: boolean
  /** 令牌声明模式：standard 只下发 OIDC 标准 claims；custom 按 token_attributes / claim_map 下发业务 claims */
  token_format: string
  token_attributes: TokenAttribute[]
  claim_map: string
  forced_redirect_origin: string
  backchannel_logout_uri: string

  // 品牌主题
  color_primary: string
  border_radius: number
  is_compact: boolean
  theme_enabled: boolean

  state: string
}

export const emptyApplicationForm: ApplicationFormValue = {
  institution_id: 0,
  code: '',
  name: '',
  category: 'default',
  type: 'oidc',
  logo: '',
  logo_dark: '',
  favicon: '',
  title: '',
  description: '',
  homepage_url: '',
  tags: [],
  cert_id: '',
  default_job_title_id: '',
  org_choice_mode: '',
  ip_restriction: '',
  ip_whitelist: '',
  signup_url: '',
  terms_of_use: '',

  signin_method_ids: [],

  self_signup_enabled: false,
  signup_auto_signin: true,
  signup_fields: [
    { name: 'Username', required: true, builtin: true },
    { name: 'Display name', required: false },
    { name: 'Email', required: true },
    { name: 'Password', required: true, builtin: true },
  ],

  max_sessions: 5,
  // 对齐 Casdoor「安全设置」的默认值：失败 5 次锁定、等待 15 分钟（0 = 沿用平台默认）
  signin_error_limit: 5,
  signin_retry_wait_minutes: 15,

  grant_types: ['authorization_code', 'refresh_token'],
  redirect_uris: [],
  scopes: [
    { name: 'openid', displayName: '', description: '' },
    { name: 'profile', displayName: '', description: '' },
    { name: 'email', displayName: '', description: '' },
  ],
  expire_in_hours: 1,
  refresh_expire_in_hours: 24,
  cookie_expire_in_hours: 720,
  disable_refresh_rotation: false,
  token_format: 'standard',
  token_attributes: [],
  claim_map: '',
  forced_redirect_origin: '',
  backchannel_logout_uri: '',

  color_primary: '',
  border_radius: 10,
  is_compact: false,
  theme_enabled: false,

  state: 'active',
}

/** 可申请的 scope 候选（与 OIDC 发现文档保持一致） */
export const scopeOptions: SelectOption[] = [
  { value: 'openid', label: 'openid', hint: 'OIDC 必需' },
  { value: 'profile', label: 'profile', hint: '姓名/头像/角色' },
  { value: 'email', label: 'email', hint: '邮箱' },
  { value: 'phone', label: 'phone', hint: '手机号' },
  { value: 'address', label: 'address', hint: '地址' },
  { value: 'offline_access', label: 'offline_access', hint: '刷新令牌' },
]

/** access_token 可内嵌声明候选（与后端 tokenFieldCatalog 对齐；部门字段为数组） */
export const tokenFieldOptions: SelectOption[] = [
  { value: 'username', label: 'username' },
  { value: 'uid', label: 'uid' },
  { value: 'display_name', label: 'display_name' },
  { value: 'email', label: 'email' },
  { value: 'email_verified', label: 'email_verified', hint: 'boolean' },
  { value: 'phone', label: 'phone' },
  { value: 'phone_verified', label: 'phone_verified', hint: 'boolean' },
  { value: 'institution', label: 'institution', hint: '组织主键' },
  { value: 'department_ids', label: 'department_ids', hint: '部门主键（数组）' },
  { value: 'departments', label: 'departments', hint: '部门路径（数组）' },
  { value: 'job_titles', label: 'job_titles', hint: '职务（数组）' },
  { value: 'title', label: 'title' },
  { value: 'is_admin', label: 'is_admin', hint: 'boolean' },
  { value: 'type', label: 'type' },
  // 早期字段名，保留兼容；新配置请使用 display_name
  { value: 'name', label: 'name', hint: 'legacy = display_name' },
]

/** 注册表单字段编排候选（顺序即注册页展示顺序） */
export const signupItemOptions: SelectOption[] = [
  { value: 'Username', label: 'Username' },
  { value: 'Display name', label: 'Display name' },
  { value: 'First name', label: 'First name' },
  { value: 'Last name', label: 'Last name' },
  { value: 'Email', label: 'Email' },
  { value: 'Phone', label: 'Phone' },
  { value: 'Password', label: 'Password' },
  { value: 'Confirm password', label: 'Confirm password' },
  { value: 'Terms of use', label: 'Terms of use' },
]

export const grantTypeOptions: SelectOption[] = [
  { value: 'authorization_code', label: 'authorization_code' },
  { value: 'password', label: 'password' },
  { value: 'refresh_token', label: 'refresh_token' },
  { value: 'client_credentials', label: 'client_credentials' },
  { value: 'implicit', label: 'implicit' },
]

/** 应用类型候选（SAML / CAS 未实现，不在候选内） */
export const appTypeOptions: SelectOption[] = [
  { value: 'oidc', label: 'OIDC / OAuth2' },
  { value: 'oauth', label: 'OAuth2 only' },
]

/** 应用标识（code）的合法形式：小写字母/数字开头，允许 - 与 _，长度 2–31 */
export const applicationCodePattern = /^[a-z0-9][a-z0-9-_]{1,30}$/

/** 回调地址校验：https（本机允许 http）、必须有主机名、不得带 fragment */
export function invalidRedirectURI(raw: string): boolean {
  const value = raw.trim()
  if (!value) return false
  try {
    const url = new URL(value.replace(/\*$/, ''))
    if (url.hash) return true
    if (!url.hostname) return true
    if (url.protocol === 'https:') return false
    if (url.protocol === 'http:') {
      return !['localhost', '127.0.0.1', '::1'].includes(url.hostname)
    }
    return true
  } catch {
    return true
  }
}

/** claimMap（{claim: userAttr}）→ 每行 "claim=userAttr" 的文本 */
function claimMapToText(claimMap?: Record<string, string>): string {
  if (!claimMap) return ''
  return Object.entries(claimMap)
    .map(([claim, attr]) => `${claim}=${attr}`)
    .join('\n')
}

/** 文本（每行 "claim=userAttr"）→ claimMap 对象；无有效行时返回 undefined */
function textToClaimMap(text: string): Record<string, string> | undefined {
  const map: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const eq = trimmed.indexOf('=')
    if (eq <= 0) continue
    const claim = trimmed.slice(0, eq).trim()
    const attr = trimmed.slice(eq + 1).trim()
    if (claim && attr) map[claim] = attr
  }
  return Object.keys(map).length > 0 ? map : undefined
}

export function applicationToForm(app: Application): ApplicationFormValue {
  return {
    ...emptyApplicationForm,
    institution_id: Number(app.institution_id),
    code: app.code,
    name: app.name ?? '',
    category: app.category || 'default',
    type: app.type || 'oidc',
    logo: app.logo ?? '',
    logo_dark: app.logo_dark ?? '',
    favicon: app.favicon ?? '',
    title: app.title ?? '',
    description: app.description ?? '',
    homepage_url: app.homepage_url ?? '',
    tags: app.tags ?? [],
    cert_id: app.cert_id ? String(app.cert_id) : '',
    default_job_title_id: app.default_job_title_id ? String(app.default_job_title_id) : '',
    org_choice_mode: app.org_choice_mode ?? '',
    ip_restriction: app.ip_restriction ?? '',
    ip_whitelist: app.ip_whitelist ?? '',
    signup_url: app.signup_url ?? '',
    terms_of_use: app.terms_of_use ?? '',

    signin_method_ids:
      app.signin_method_ids ??
      (app.signin_methods ?? [])
        .map((method) => method.id)
        .filter((id): id is number => typeof id === 'number'),

    self_signup_enabled: app.self_signup_enabled ?? false,
    signup_auto_signin: app.signup_auto_signin ?? true,
    signup_fields: signupFieldsFromApplication(app),

    max_sessions: app.max_sessions ?? 5,
    signin_error_limit: app.signin_error_limit ?? 5,
    signin_retry_wait_minutes: app.signin_retry_wait_minutes ?? 15,

    grant_types: app.grant_types ?? [],
    redirect_uris: app.redirect_uris ?? [],
    scopes:
      (app.scopes ?? []).length > 0
        ? (app.scopes ?? []).map((item) => ({
            name: item.name,
            displayName: item.displayName ?? '',
            description: item.description ?? '',
          }))
        : emptyApplicationForm.scopes.map((row) => ({ ...row })),
    expire_in_hours: app.expire_in_hours ?? 1,
    refresh_expire_in_hours: app.refresh_expire_in_hours ?? 24,
    cookie_expire_in_hours: app.cookie_expire_in_hours ?? 720,
    disable_refresh_rotation: app.disable_refresh_rotation ?? false,
    // 历史应用可能仍是 jwt：按 custom 处理，避免升级后静默丢掉业务 claim。
    token_format: app.token_format === 'standard' ? 'standard' : 'custom',
    token_attributes: app.token_attributes ?? [],
    claim_map: claimMapToText(app.adapter_config?.claimMap),
    forced_redirect_origin: app.forced_redirect_origin ?? '',
    backchannel_logout_uri: app.backchannel_logout_uri ?? '',

    color_primary: app.theme_data?.colorPrimary ?? '',
    border_radius: app.theme_data?.borderRadius ?? 10,
    is_compact: app.theme_data?.isCompact ?? false,
    theme_enabled: app.theme_data?.isEnabled ?? false,

    state: app.state || 'active',
  }
}

/** 表单 → 提交载荷（与后端实体字段一一对应；不含 client_id / client_secret） */
export function formToApplication(form: ApplicationFormValue) {
  const claimMap = textToClaimMap(form.claim_map)
  return {
    institution_id: Number(form.institution_id) || 0,
    code: form.code.trim(),
    name: form.name.trim(),
    category: form.category,
    type: form.type,
    logo: form.logo,
    logo_dark: form.logo_dark,
    favicon: form.favicon,
    title: form.title,
    description: form.description,
    homepage_url: form.homepage_url,
    tags: form.tags,
    cert_id: form.cert_id ? Number(form.cert_id) : null,
    default_job_title_id: form.default_job_title_id ? Number(form.default_job_title_id) : null,
    org_choice_mode: form.org_choice_mode,
    ip_restriction: form.ip_restriction,
    ip_whitelist: form.ip_whitelist,
    signup_url: form.signup_url,
    terms_of_use: form.terms_of_use,

    signin_method_ids: form.signin_method_ids,

    self_signup_enabled: form.self_signup_enabled,
    signup_auto_signin: form.signup_auto_signin,
    signup_items: form.signup_fields.map((row) => ({
      name: row.name,
      visible: true,
      required: row.required,
    })),

    max_sessions: Number(form.max_sessions) || 0,
    // 0 保持 0（= 沿用平台默认），负数由校验拦下
    signin_error_limit: Number(form.signin_error_limit) || 0,
    signin_retry_wait_minutes: Number(form.signin_retry_wait_minutes) || 0,

    grant_types: form.grant_types,
    redirect_uris: form.redirect_uris.map((item) => item.trim()).filter(Boolean),
    // 名称为空的行视为未填写，直接忽略（避免写入空 scope）
    scopes: form.scopes
      .filter((row) => row.name.trim() !== '')
      .map((row) => ({
        name: row.name.trim(),
        displayName: row.displayName.trim() || row.name.trim(),
        description: row.description.trim() || undefined,
      })),
    expire_in_hours: Number(form.expire_in_hours) || 1,
    refresh_expire_in_hours: Number(form.refresh_expire_in_hours) || 24,
    cookie_expire_in_hours: Number(form.cookie_expire_in_hours) || 720,
    disable_refresh_rotation: form.disable_refresh_rotation,
    token_format: form.token_format,
    token_attributes: form.token_attributes,
    // 适配器开关由 claimMap 是否为空推导，避免与「标准/自定义令牌格式」形成两套语义。
    adapter: claimMap ? 'custom' : 'default',
    adapter_config: claimMap ? { claimMap } : null,
    forced_redirect_origin: form.forced_redirect_origin,
    backchannel_logout_uri: form.backchannel_logout_uri,
    theme_data: form.theme_enabled
      ? {
          colorPrimary: form.color_primary,
          borderRadius: Number(form.border_radius) || 10,
          isCompact: form.is_compact,
          isEnabled: true,
        }
      : null,
    state: form.state || 'active',
  }
}

/** 提交前校验：返回 { ok, reason }，reason 为 i18n key（供表单就地提示） */
export function formToApplicationValidation(form: ApplicationFormValue): {
  ok: boolean
  reason?: string
} {
  if (!applicationCodePattern.test(form.code.trim())) {
    return { ok: false, reason: 'applications.codeInvalid' }
  }
  if (form.signin_method_ids.length === 0) {
    return { ok: false, reason: 'applications.methodsEmpty' }
  }
  // scope：填了显示名/描述却没填名称 → 提示；名称重复 → 拒绝
  if (
    form.scopes.some((row) => row.name.trim() === '' && (row.displayName.trim() || row.description.trim()))
  ) {
    return { ok: false, reason: 'applications.scopeNameRequired' }
  }
  const scopeNames = form.scopes.map((row) => row.name.trim()).filter(Boolean)
  if (new Set(scopeNames).size !== scopeNames.length) {
    return { ok: false, reason: 'applications.scopeDuplicated' }
  }
  if (form.self_signup_enabled && form.signup_fields.length === 0) {
    return { ok: false, reason: 'applications.signupFieldsRequired' }
  }
  if (form.signin_error_limit < 0 || form.signin_retry_wait_minutes < 0) {
    return { ok: false, reason: 'applications.signinLimitNegative' }
  }
  const redirects = form.redirect_uris.map((item) => item.trim()).filter(Boolean)
  if (form.grant_types.includes('authorization_code')) {
    if (redirects.length === 0) {
      return { ok: false, reason: 'applications.redirectRequired' }
    }
    if (redirects.some((item) => invalidRedirectURI(item))) {
      return { ok: false, reason: 'applications.redirectInvalid' }
    }
  }
  return { ok: true }
}

/** 预览用：按组织方法顺序返回应用已选中的方式类型。 */
export function selectedSigninMethodKinds(
  form: ApplicationFormValue,
  methods: { id: number; name: string }[],
): string[] {
  const selected = new Set(form.signin_method_ids)
  return methods.filter((method) => selected.has(method.id)).map((method) => method.name)
}

/**
 * Application -> 注册字段行。
 * 内置字段（Username / Password）始终存在：历史数据缺失时补回，且必填锁定。
 */
function signupFieldsFromApplication(app: Application): SignupFieldRow[] {
  const rows: SignupFieldRow[] = (app.signup_items ?? [])
    .filter((item) => item.visible !== false)
    .map((item) => ({
      name: item.name,
      required: Boolean(item.required),
      builtin: (SIGNUP_BUILTIN_FIELDS as readonly string[]).includes(item.name),
    }))
  if (rows.length === 0) {
    return emptyApplicationForm.signup_fields.map((row) => ({ ...row }))
  }
  // 内置字段：存在则锁定必填；不存在则补回（Username 置首、Password 置尾）
  for (const builtin of SIGNUP_BUILTIN_FIELDS) {
    const existing = rows.find((row) => row.name === builtin)
    if (existing) {
      existing.builtin = true
      existing.required = true
      continue
    }
    const row: SignupFieldRow = { name: builtin, required: true, builtin: true }
    if (builtin === 'Username') rows.unshift(row)
    else rows.push(row)
  }
  return rows
}

/**
 * 应用访问授权（向导第 6 步「授权与访问」）。
 *
 * 刻意**不放进 ApplicationFormValue**：授权由独立端点读写（PUT /applications/:id/access），
 * 与应用主表单整行更新是两条写入路径，混在一起会让"零值覆盖"再次发生（历史教训）。
 */
export interface ApplicationAccessFormValue {
  /** all = 全员可访问（默认）| restricted = 受限（仅名单内主体可访问） */
  mode: string
  userIds: number[]
  jobTitleIds: number[]
  departmentIds: number[]
}

export const emptyApplicationAccess: ApplicationAccessFormValue = {
  mode: 'all',
  userIds: [],
  jobTitleIds: [],
  departmentIds: [],
}

/** 接口返回的授权全景 → 表单值 */
export function accessToFormValue(access: ApplicationAccess): ApplicationAccessFormValue {
  return {
    mode: access.access_mode === 'restricted' ? 'restricted' : 'all',
    userIds: (access.users ?? []).map((item) => item.id),
    jobTitleIds: (access.groups ?? []).map((item) => item.id),
    departmentIds: (access.departments ?? []).map((item) => item.id),
  }
}
