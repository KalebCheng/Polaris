import { api } from '@/shared/api/client'
import type {
  AppInfo,
  AuthStatus,
  CaptchaChallenge,
  LoginResponse,
  MeResponse,
  ProviderInfo,
} from '@/shared/api/types'

export interface AppAccessResult {
  allowed: boolean
  reason?: 'app_access_denied' | 'application_disabled' | 'institution_mismatch' | string
  application_code?: string
  application_name?: string
}

export interface LoginPayload {
  /** 组织主键（权威引用）；institution 仅作兼容回落 */
  institution_id?: number
  institution?: string
  application?: string
  /** 应用驱动入口的应用上下文（URL 的 client_id），优先于 application */
  client_id?: string
  username: string
  password: string
  /** 密码标识类型：username | email | phone（应用只允许一种时可省略） */
  identifier_type?: 'username' | 'email' | 'phone'
  mfa_code?: string
  /** 勾选「记住此设备」（组织配置了记住时长时生效） */
  remember_mfa?: boolean
  from?: string
  captcha_id?: string
  captcha_code?: string
  captcha_token?: string
}

export const authApi = {
  login: (body: LoginPayload) => api.post<LoginResponse>('/auth/login', body),
  logout: () => api.post<void>('/auth/logout'),
  status: () => api.get<AuthStatus>('/auth/status'),
  /** 当前用户详情（用于个人资料页展示 MFA 等状态） */
  me: () => api.get<MeResponse>('/me'),
  providers: (institutionId?: number, institution?: string, application?: string, clientId?: string) =>
    api.get<{ providers: ProviderInfo[] | null }>('/auth/providers', {
      institution_id: institutionId,
      institution,
      application,
      client_id: clientId,
    }),
  /** 登录页公开的应用策略（登录方式、字段编排、是否需要人机校验） */
  appInfo: (institutionId?: number, institution?: string, application?: string) =>
    api.get<AppInfo>('/auth/app-info', { institution_id: institutionId, institution, application }),
  /** 按 client_id 获取应用策略（应用驱动登录页） */
  appInfoByClientId: (clientId: string) => api.get<AppInfo>('/auth/app-info', { client_id: clientId }),
  /** 登录页预检：当前会话用户能否访问该应用（避免未授权时被直接送到 RP 的 JSON 错误页） */
  appAccess: (clientId: string) => api.get<AppAccessResult>('/auth/app-access', { client_id: clientId }),
  /** 人机校验挑战（组织未启用验证码提供商时 required=false） */
  captcha: (institutionId?: number, institution?: string) =>
    api.get<CaptchaChallenge>('/auth/captcha', { institution_id: institutionId, institution }),
  /** 发送邮箱/短信登录验证码 */
  sendCode: (body: {
    institution_id?: number
    institution?: string
    application?: string
    /** 应用上下文：应用驱动登录页必带，否则发码/登录会落到"组织默认应用" */
    client_id?: string
    kind: 'email' | 'sms'
    target: string
    captcha_id?: string
    captcha_code?: string
    captcha_token?: string
  }) => api.post<{ status: string }>('/auth/codes', body),
  /** 发送邮件魔法链接（免密登录）：无论邮箱是否存在都返回 sent（防用户枚举） */
  sendMagicLink: (body: {
    institution_id?: number
    institution?: string
    application?: string
    client_id?: string
    email: string
    from?: string
    captcha_id?: string
    captcha_code?: string
    captcha_token?: string
  }) => api.post<{ status: string }>('/auth/magic-link', body),
  /** 校验魔法链接换取会话（token 一次性消费） */
  verifyMagicLink: (body: { token: string; client_id?: string }) =>
    api.post<LoginResponse>('/auth/magic-link/verify', body),
  /** 自助注册（字段由应用 signup_items 编排驱动） */
  signup: (body: {
    institution_id?: number
    institution?: string
    application?: string
    client_id?: string
    name: string
    display_name?: string
    password: string
    email?: string
    phone?: string
    from?: string
    fields?: Record<string, string>
    captcha_id?: string
    captcha_code?: string
    captcha_token?: string
  }) => api.post<LoginResponse>('/auth/signup', body),
  /** 验证码登录 */
  codeLogin: (body: {
    institution?: string
    application?: string
    /** 应用上下文：应用驱动登录页必带 */
    client_id?: string
    kind: 'email' | 'sms'
    target: string
    code: string
    from?: string
  }) => api.post<LoginResponse>('/auth/login/code', body),
  /** 登录页可选组织（公开端点，仅名称与显示名） */
  institutions: () =>
    api.get<{ institutions: { id: number; name: string; display_name: string }[] | null }>(
      '/auth/institutions',
    ),
  // client_id 随 state 暂存：SSO 回跳后的应用准入判定需要知道"这是哪个应用的入口"
  oauthAuthorize: (provider: string, from?: string, institutionId?: number, clientId?: string) =>
    api.get<{ authorization_url: string; state: string }>(`/auth/oauth/${provider}/authorize`, {
      from,
      institution_id: institutionId,
      client_id: clientId,
    }),
  // 默认回调页 /sso/callback 不带 provider；provider 由后端从 state 载荷解析。
  oauthCallback: (code: string, state: string) =>
    api.get<LoginResponse>('/auth/oauth/callback', { code, state }),
  // 账号尚未设置密码时可不传 old_password（首次设密，如第三方登录自动建号）
  changePassword: (body: { old_password?: string; new_password: string }) =>
    api.post<void>('/auth/change-password', body),
  mfaOptions: () => api.get<MFAOptions>('/auth/mfa/options'),
  mfaEnroll: (providerID: number) =>
    api.post<MFAEnrollResult>('/auth/mfa/enroll', { provider_id: providerID }),
  mfaResendEnroll: (providerID: number) =>
    api.post<{ status: string }>('/auth/mfa/enroll/resend', { provider_id: providerID }),
  mfaEnrollVerify: (providerID: number, code: string) =>
    api.post<{ recovery_codes?: string[] | null }>('/auth/mfa/enroll/verify', {
      provider_id: providerID,
      code,
    }),
  mfaVerify: (providerID: number, code: string, remember = false) =>
    api.post<{ status: string }>('/auth/mfa/verify', { provider_id: providerID, code, remember }),
  mfaSkip: () => api.post<{ status: string }>('/auth/mfa/skip'),
  mfaDisable: (providerID: number) =>
    api.post<{ status: string }>('/auth/mfa/disable', { provider_id: providerID }),
}

export interface MFAProviderOption {
  id: number
  type: 'totp' | 'sms' | string
  name: string
  display_name: string
}

export interface MFABoundOption {
  provider_id: number
  type: 'totp' | 'sms' | string
  name: string
  display_name: string
}

export interface MFAOptions {
  required: boolean
  can_skip: boolean
  providers: MFAProviderOption[] | null
  bound: MFABoundOption[] | null
  remember_hours: number
}

export interface MFAEnrollResult {
  provider_id: number
  type: 'totp' | 'sms' | string
  secret?: string
  url?: string
  sent?: boolean
  phone?: string
  ttl_seconds?: number
}
