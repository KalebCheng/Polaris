/** 后端统一响应包装 */
export interface ApiResponse<T> {
  code: string
  message: string
  data?: T
  details?: Record<string, unknown>
}

/** 分页列表响应 */
export interface PageData<T> {
  items: T[]
  total: number
  page: number
  size: number
}

export interface Institution {
  id: number
  name: string
  display_name: string
  domain?: string
  website_url?: string
  logo?: string
  favicon?: string
  description?: string
  captcha?: string
  password_type: string
  /** 是否已配置组织级密码盐（write-only：响应只回布尔，不回明文） */
  has_password_salt: boolean
  password_options?: string[]
  password_expire_days: number
  password_history_count: number
  token_retention_days: number
  record_retention_days: number
  default_application?: string
  use_email_as_username: boolean
  disable_signin: boolean
  mfa_policy: string
  mfa_remember_hours: number
  status: string
  created_at: string
  updated_at: string
}

/**
 * 组织写入载荷：password_salt 为 write-only（后端响应只回 has_password_salt，不回明文）。
 */
export type InstitutionInput = Partial<Institution> & { password_salt?: string }

/** 部门（租户内的层级节点；仅用于用户归属与筛选） */
export interface Department {
  id: number
  institution_id: number
  name: string
  display_name: string
  description?: string
  parent_id?: number | null
  code_path: string
  name_path: string
  level: number
  has_child: boolean
  sort_index: number
  created_at: string
  updated_at: string
}

export interface User {
  /** 多值部门归属（Spec Q2 = A；仅详情端点下发） */
  department_ids?: number[]
  /** 全部部门归属的展示路径（与 department_ids 同序） */
  department_paths?: string[]
  /** 职级（Spec Q4.1 = A：仅属性） */
  job_level_id?: number | null
  /** 职级名称（服务端补全，列表/详情直接展示） */
  job_level?: string
  id: number
  uid: string
  institution_id: number
  name: string
  type: string
  department_id?: number | null
  /** 部门展示路径（服务端按当前组织批量补全；未归属部门时不下发） */
  department_path?: string
  display_name?: string
  first_name?: string
  last_name?: string
  avatar?: string
  email?: string
  email_verified: boolean
  phone?: string
  country_code?: string
  phone_verified: boolean
  location?: string
  address?: string
  affiliation?: string
  title?: string
  homepage?: string
  bio?: string
  language?: string
  gender?: string
  birthday?: string
  education?: string
  score: number
  is_admin: boolean
  is_forbidden: boolean
  is_deleted: boolean
  is_online: boolean
  mfa_enabled: boolean
  has_password: boolean
  register_type?: string
  register_source?: string
  last_signin_time?: string | null
  last_signin_ip?: string
  created_at: string
  updated_at: string
}

/** 用户安全状态：密码与 MFA（会话与锁定条目分别来自各自的接口） */
export interface UserSecurityStatus {
  has_password: boolean
  password_updated_at?: string | null
  /** 依据组织密码有效期策略推算（策略为 0 时不下发） */
  password_expire_at?: string | null
  mfa_enabled: boolean
  mfa_type?: string
  /** 恢复码剩余数量（只回数量，不回码本身） */
  mfa_recovery_codes_left: number
}

/** 一条登录失败计数记录（按应用 + IP 维度） */
export interface LoginLockout {
  /** 触发计数的应用 code；空串表示该组织未配置应用 */
  application: string
  ip: string
  failures: number
  frozen_seconds_left: number
  /** 是否已达到生效阈值（达到即登录返回 429） */
  blocked: boolean
}

export interface UserSecurity {
  password: UserSecurityStatus
  lockouts: LoginLockout[]
}

/** 单条有效权限（含来源标注：user / role:<角色> / job_title:<职务>） */
export interface EffectivePermission {
  name?: string
  resource: string
  action: string
  effect: string
  sources: string[]
}

export interface UserAuthorizations {
  direct_roles: string[]
  inherited_roles: string[]
  job_titles: string[]
  direct_permissions: string[]
  effective_permissions: EffectivePermission[]
}

/** 用户资料部分更新入参（未出现的字段一律不修改） */
export interface UserProfileInput {
  display_name?: string
  first_name?: string
  last_name?: string
  avatar?: string
  email?: string
  email_verified?: boolean
  phone?: string
  phone_verified?: boolean
  country_code?: string
  location?: string
  address?: string
  affiliation?: string
  title?: string
  homepage?: string
  bio?: string
  language?: string
  gender?: string
  birthday?: string
  education?: string
}

/** 职务详情：成员明细 + 关联角色 + 汇总权限 */
export interface JobTitleDetail {
  jobTitle: JobTitle
  members: User[]
  roles: string[]
  permissions: Permission[]
}

/** 外部身份映射（身份连接侧；raw 为源端报文） */
export interface ExternalIdentityRef {
  syncer_id: number
  syncer_name: string
  syncer_type: string
  external_id: string
  raw?: string
  updated_at: string
}

export interface UserIdentity {
  id: number
  institution_id: number
  user: string
  provider: string
  provider_user_id: string
  email?: string
  created_at: string
}

/** 系统级默认对象存储配置（Secret Key 为 write-only，只回 has_secret） */
export interface StorageSettings {
  type: string
  endpoint?: string
  region?: string
  bucket?: string
  access_key?: string
  path_prefix?: string
  domain?: string
  root_path?: string
  public_base?: string
  path_style: boolean
  has_secret: boolean
  updated_at: string
}

export type StorageSettingsInput = Partial<StorageSettings> & { secret_key?: string }

export interface ThemeData {
  themeType?: string
  colorPrimary?: string
  borderRadius?: number
  isCompact?: boolean
  isEnabled?: boolean
}

/** 应用适配器配置（P4）：claim 名 → 用户属性名 */
export interface AdapterConfig {
  claimMap?: Record<string, string>
}

export interface ProviderItem {
  name: string
  canSignUp: boolean
  canSignIn: boolean
  canUnlink?: boolean
  prompted?: boolean
  alertType?: string
  rule?: string
}

export interface ScopeItem {
  name: string
  displayName: string
  description?: string
}

export interface SigninMethod {
  name: string
  displayName: string
  rule?: string
}

export interface SignupItem {
  name: string
  visible: boolean
  required: boolean
  prompted?: boolean
  type?: string
  label?: string
  placeholder?: string
  options?: string[]
  regex?: string
  rule?: string
}

/** 应用（OAuth2 / OIDC 客户端）——覆盖平台运行时真实读取的全部字段 */
export interface Application {
  id: number
  institution_id: number
  /** 应用标识：进入 URL 与外部集成，创建后不可修改（仅小写字母/数字/-/_） */
  code: string
  /** 应用名称：人类可读，可修改 */
  name: string
  category: string
  type: string
  logo?: string
  logo_dark?: string
  favicon?: string
  title?: string
  description?: string
  homepage_url?: string
  order: number
  tags?: string[]
  cert?: string
  default_group?: string
  theme_data?: ThemeData | null

  // 登录页行为与访问限制
  org_choice_mode?: string
  ip_restriction?: string
  ip_whitelist?: string

  /** 访问模式：all = 全员可访问（默认）；restricted = 仅授权名单内主体（含其子部门） */
  access_mode: string
  /** 授权主体数（仅列表返回；详情走 GET /applications/:id/access） */
  access_subject_count?: number

  // 跳转地址
  signup_url?: string
  terms_of_use?: string

  // 登录方式（唯一数据源）与注册编排
  signin_methods?: SigninMethod[]
  signup_items?: SignupItem[]
  self_signup_enabled: boolean
  signup_auto_signin: boolean
  email_code_ttl_seconds: number
  sms_code_ttl_seconds: number
  magic_link_ttl_seconds: number

  // 会话策略：0 = 不限制；1 = 独占（新登录踢掉其他会话）；N = 最多 N 个
  max_sessions: number

  // 登录失败限制：0 = 沿用平台默认（auth.rate_limit）
  signin_error_limit: number
  signin_retry_wait_minutes: number

  grant_types: string[]
  redirect_uris: string[]
  providers: ProviderItem[]
  scopes: ScopeItem[]

  client_id: string
  /** 密钥不下发明文：仅以 has_client_secret 表达“是否已配置” */
  has_client_secret: boolean

  token_format: string
  token_signing_method: string
  token_fields?: string[]
  expire_in_hours: number
  refresh_expire_in_hours: number
  disable_refresh_rotation: boolean
  cookie_expire_in_hours: number
  forced_redirect_origin?: string
  backchannel_logout_uri?: string

  /** 应用适配器（P4）：default 标准 claims；custom 时以 adapter_config.claimMap 驱动 */
  adapter?: string
  adapter_config?: AdapterConfig | null

  state: string
  created_at: string
  updated_at: string
}

/** 创建应用响应：唯一一次性返回明文密钥的时机之一（另一次是重置密钥） */
/** 授权主体（应用准入名单的一条；missing = 主体已被删除，判定不命中） */
export interface ApplicationSubjectView {
  name: string
  display_name: string
  /** 层级路径：部门 name_path / 职务（扁平，无父级） */
  path?: string
  email?: string
  missing: boolean
}

/** 应用的访问授权全景 */
export interface ApplicationAccess {
  access_mode: string
  users: ApplicationSubjectView[]
  groups: ApplicationSubjectView[]
  departments: ApplicationSubjectView[]
}

/** 可授权主体候选（选择器数据源） */
export interface ApplicationSubjectOption {
  name: string
  display_name: string
  path?: string
  /** 仅部门：展开该节点时用它作为 parent_id 再取一层 */
  id?: number
  /** 仅部门：是否还有下级（决定是否画展开箭头） */
  has_child?: boolean
}

/** 候选主体分页结果（total 供分页器 / 「加载更多」判断是否还有更多） */
export interface ApplicationSubjectOptionPage {
  items: ApplicationSubjectOption[]
  total: number
}

export interface ApplicationAccessInput {
  access_mode: string
  users: string[]
  /** 职务主体名单（后端 JSON 键仍为 groups；主体类型取值 'job_title'） */
  groups: string[]
  departments: string[]
}

export interface ApplicationCreated extends Application {
  client_secret: string
  endpoints: {
    issuer: string
    authorize: string
    token: string
    userinfo: string
    jwks: string
  }
}

/** 登录页公开的应用信息（GET /auth/app-info） */
export interface AppInfo {
  /** 组织标识（组织 name）：兼容字段，仅用于展示/回落 */
  institution: string
  /** 组织主键：公开面的**权威引用**（登录/发码/授权请求都传它） */
  institution_id: number
  /** 应用标识 */
  code: string
  /** 应用名称（人类可读） */
  name: string
  logo?: string
  favicon?: string
  title?: string
  theme_data?: ThemeData | null
  /** 登录方式：后端已按组织能力过滤（不可用的方式不会出现） */
  signin_methods: SigninMethod[]
  self_signup_enabled: boolean
  signup_auto_signin: boolean
  magic_link_ttl_seconds: number
  org_choice_mode?: string
  sms_providers: string[]
  email_providers: string[]
  providers: ProviderItem[]
  signup_items: SignupItem[]
  signup_url?: string
  terms_of_use?: string
  captcha_required: boolean
  /** 组织展示名（登录页左上角品牌名取自组织，而非平台名） */
  org_display_name?: string
  /** 组织描述（登录页欢迎语） */
  org_description?: string
  /** 组织 Logo（登录页左上角品牌标识） */
  org_logo?: string
  /** 组织 Favicon（登录页浏览器页签图标） */
  org_favicon?: string
}

export interface CaptchaChallenge {
  required: boolean
  type?: string
  site_key?: string
  captcha_id?: string
  image?: string
}

/** 提供商字段控件类型（后端目录声明） */
export type ProviderFieldType =
  | 'text'
  | 'password'
  | 'number'
  | 'switch'
  | 'select'
  | 'tags'
  | 'textarea'
  /** 受限多选：取值必须来自字段声明的 options（如短信模板变量） */
  | 'multiselect'

/** 提供商表单字段（GET /providers/catalog 下发，前端据此渲染向导） */
export interface ProviderField {
  key: string
  label: string
  /** 归属的向导步骤：basic | connection | advanced */
  group: string
  type: ProviderFieldType
  required?: boolean
  /** 创建后不可修改（编辑态只读） */
  immutable?: boolean
  /** 敏感字段：只在 secrets 桶提交，响应只回 has_secret */
  secret?: boolean
  /** 策略字段：归入 policy 桶（SSO 信任策略） */
  policy?: boolean
  placeholder?: string
  hint?: string
  options?: { value: string; label: string }[]
  default?: string
}

/** 提供商类型能力声明（决定界面展示哪些配置与动作） */
export interface ProviderCapabilities {
  /** 可作为应用登录方式（外部身份提供方） */
  signin: boolean
  /** 通知类（邮件 / 短信） */
  notify: boolean
  captcha: boolean
  storage: boolean
  /** 支持连通性自检 */
  test: boolean
}

/** 提供商类型（类别下的一个具体实现） */
export interface ProviderTypeInfo {
  key: string
  name: string
  description: string
  icon: string
  category: string
  fields: ProviderField[]
  capabilities: ProviderCapabilities
}

/** 提供商类别定义（GET /providers/catalog） */
export interface ProviderCategory {
  key: string
  name: string
  description: string
  /** 类别图标 key（与类型图标同一套，由 ProviderIcon 渲染） */
  icon: string
  /** 同一组织内该类别最多只能有一个启用项（邮件/短信/人机校验/存储为 true；外部身份提供方为 false） */
  singleton: boolean
  types: ProviderTypeInfo[]
}

/** 提供商目录（类别 → 类型 → 字段 schema；后端唯一来源） */
export interface ProviderCatalog {
  categories: ProviderCategory[]
}

/** 组织资源聚合统计 */
export interface InstitutionStats {
  /**
   * 组织**标识**（`institutions.name`，如 `built-in`）。
   * 注意：后端 `/institutions/stats` 目前下发的键名是驼峰 `institutionID`（历史契约），
   * 与 §3.7「JSON tag 一律蛇形」不符——改它属后端契约变更，需单独一批（含冒烟断言）。
   * 前端此处按**真实下行**声明，避免出现 `undefined` 的 key 与 `/institutions/undefined` 链接。
   */
  institutionID: string
  display_name?: string
  counts: Record<string, number>
  total: number
  last_activity?: string
}

export interface Provider {
  id: number
  institution_id: number
  name: string
  display_name: string
  /** oauth | email | sms | captcha | storage */
  category: string
  /** 类型 key（如 dingtalk / oidc / smtp / aliyun / image / local） */
  type: string
  state: string
  /** 非敏感配置（键由目录声明） */
  config: Record<string, unknown>
  /** SSO 信任策略（仅外部身份提供方） */
  policy: Record<string, unknown>
  /** 是否已配置密钥（响应绝不下发明文或密文） */
  has_secret: boolean
  created_at: string
  updated_at: string
}

export interface Role {
  id: number
  institution_id: number
  name: string
  display_name: string
  description: string
  is_enabled: boolean
  is_system: boolean
  users?: string[]
  job_titles?: string[]
  roles?: string[]
  created_at: string
  updated_at: string
}

export interface Permission {
  id: number
  institution_id: number
  name: string
  display_name: string
  description: string
  users?: string[]
  job_titles?: string[]
  roles?: string[]
  resource_type: string
  resources: string[]
  actions: string[]
  effect: string
  is_enabled: boolean
  expire_time?: string | null
  state: string
  created_at: string
  updated_at: string
}

/** 职级（Spec 第六阶段 Q4）：仅作用户属性（展示 / 筛选 / 统计），不参与授权 */
export interface JobLevel {
  id: number
  institution_id: number
  name: string
  description: string
  /** 对齐飞书 order 语义：数值越小越靠前 */
  sort_order: number
  is_enabled: boolean
  created_at: string
  updated_at: string
}

export interface JobTitle {
  id: number
  institution_id: number
  name: string
  display_name: string
  manager?: string
  contact_email?: string
  type?: string
  parent_id?: number | null
  is_top_group: boolean
  is_enabled: boolean
  properties: Record<string, string>
  users?: string[]
  /** 成员数（列表接口批量下发，避免逐行查询） */
  member_count?: number
  created_at: string
  updated_at: string
}

export interface Cert {
  id: number
  institution_id: number
  name: string
  display_name: string
  scope: string
  type: string
  crypto_algorithm: string
  bit_size: number
  expire_in_years: number
  certificate?: string
  has_private_key: boolean
  expire_time?: string | null
  is_system: boolean
  created_at: string
  updated_at: string
}

export interface Token {
  id: number
  institution_id: number
  application?: string
  user?: string
  name: string
  scope?: string
  token_type: string
  grant_type?: string
  expires_in: number
  revoked: boolean
  has_access_token: boolean
  has_refresh_token: boolean
  has_id_token: boolean
  expire_time?: string | null
  created_at: string
}

export interface SessionInfo {
  id: string
  user_id: number
  institution_id: number
  name: string
  email?: string
  is_admin: boolean
  roles?: string[]
  application?: string
  ip?: string
  user_agent?: string
  created_at: number
  expires_at: number
  is_current?: boolean
}

export interface AuditRecord {
  id: number
  institution_id: number
  user?: string
  client_ip?: string
  method?: string
  request_uri?: string
  action?: string
  object?: string
  status_code: number
  detail?: string
  trace_id?: string
  created_at: string
}

export interface Syncer {
  id: number
  institution_id: number
  name: string
  display_name: string
  type: string
  sub_type?: string
  is_enabled: boolean
  schedule: string
  host?: string
  port: number
  user?: string
  database?: string
  table?: string
  database_type?: string
  ssl_mode?: string
  app_id?: string
  corp_id?: string
  user_mapping: Record<string, string>
  group_mapping: Record<string, string>
  sync_users: boolean
  sync_groups: boolean
  soft_delete: boolean
  has_secret: boolean
  has_app_secret: boolean
  last_run_time?: string | null
  last_status: string
  last_message: string
  last_counts?: string | null
  created_at: string
  updated_at: string
}

/** 同步运行统计（后端 counts 的 JSON 解码结果；不再下发 base64） */
export interface SyncerRunCounts {
  created: number
  updated: number
  skipped: number
  failed: number
  deleted: number
}

/** 试运行预览（不写库）：将创建/更新/跳过的账号与组织数量，以及同步保护是否触发 */
export interface SyncerPreview {
  user_create: number
  user_update: number
  user_skip: number
  user_disable: number
  dept_create: number
  dept_update: number
  samples?: string[]
  warnings?: string[]
  guard_tripped: boolean
  guard_message?: string
}

export interface SyncerRun {
  id: number
  status: string
  trigger?: string
  message?: string
  counts?: SyncerRunCounts
  errors?: string[]
  duration_ms?: number
  started_at: string
  finished_at: string
}

/** 下游应用连接器（推送型供给） */
export interface Connector {
  id: number
  institution_id: number
  application?: string
  name: string
  display_name: string
  /** 连接器类型（http | dingtalk | lark | wecom | ldap），决定使用哪个供给实现 */
  type: string
  /** 类型专属的非敏感配置（键由类型目录声明，如 provider_url / app_id / dept_id / host …） */
  config: Record<string, string>
  /** 敏感键是否已配置（口令/密钥一律不下发明文） */
  has_secrets: Record<string, boolean>
  state: string
  last_run_time?: string | null
  last_status: string
  last_message?: string
  created_at: string
  updated_at: string
}

/** 账号策略（本地→下游属性映射 + 建号/回收触发策略） */
export interface AccountStrategy {
  id: number
  institution_id: number
  application_id?: number
  application?: string
  name: string
  display_name: string
  is_enabled: boolean
  mapping: Record<string, string>
  policy: Record<string, string>
  created_at: string
  updated_at: string
}

/** 应用账号（本地用户 × 应用 → 下游账号） */
export interface AppAccount {
  id: number
  institution_id: number
  application_id?: number
  application: string
  user: string
  username: string
  related_username?: string
  has_password: boolean
  create_type: string
  status: string
  created_at: string
  updated_at: string
}

/** 供给历史 */
export interface ProvisionHistory {
  id: number
  institution_id: number
  connector?: string
  application_id?: number
  application?: string
  user?: string
  action: string
  status: string
  message?: string
  created_at: string
}

export interface ProviderInfo {
  name: string
  display_name: string
  /** 提供商类型（dingtalk / wechat / feishu / lark / oidc）：登录页据此渲染品牌 logo */
  type?: string
  /** 图标 key（由后端提供商目录声明） */
  icon?: string
}

export interface PermissionRule {
  name?: string
  resource: string
  action: string
  effect: string
}

/** GET /me 响应：当前用户 + 角色 + 有效权限 */
export interface MeResponse {
  user: User
  roles: string[] | null
  permissions: PermissionRule[] | null
}

export interface AuthStatus {
  authenticated: boolean
  institution_id: number
  name?: string
  display_name?: string
  email?: string
  is_admin?: boolean
  roles?: string[]
  permissions?: PermissionRule[]
  expires_at?: number
}

export interface LoginResponse {
  session: { expires_at: number; from?: string }
  user: {
    institution_id: number
    name: string
    display_name: string
    email: string
    is_admin: boolean
    mfa_enabled: boolean
  }
  roles?: string[]
  mfa?: { required: boolean; type: string; enroll?: boolean; remember_hours?: number }
  identity_linked?: boolean
}

/** 身份连接：方向（入=从外部拉取；出=推送给下游应用） */
export type ConnectionDirection = 'in' | 'out'

export type ConnectionFieldType =
  | 'text'
  | 'password'
  | 'number'
  | 'switch'
  | 'select'
  /** 多值标签（如"每天多个时刻"） */
  | 'tags'
  /** 组织（部门树）单选 */
  | 'department'

export interface ConnectionFieldOption {
  value: string
  label: string
}

/** 表单字段所属的向导步骤（后端声明，前端据此分步渲染） */
export type ConnectionFieldGroup = 'basic' | 'connection' | 'scope'

/** 表单字段（后端 catalog 下发的 schema，前端按此渲染，禁止自行硬编码类型字段） */
export interface ConnectionField {
  key: string
  label: string
  /** 归属步骤：基本信息 / 连接配置 / 同步范围与计划 */
  group: ConnectionFieldGroup
  /** 落库路径（点分）；为空表示与 key 同名。如 sync_config.schedule.mode */
  store?: string
  type: ConnectionFieldType
  required?: boolean
  secret?: boolean
  has_value?: boolean
  placeholder?: string
  hint?: string
  options?: ConnectionFieldOption[]
  default?: string
  show_when?: { field: string; equals: string }
  /** 仅当 schedule_mode 等于该值时展示（后端便捷写法） */
  show_when_mode?: string
}

/** 同一实现下的接入变体（AD / OpenLDAP、MySQL / PostgreSQL、飞书 / Lark） */
export interface ConnectionVariant {
  key: string
  name: string
  description: string
  icon: string
}

export interface ConnectionCapabilities {
  mapping: boolean
  incremental: boolean
  group_scope: boolean
  soft_delete: boolean
  test: boolean
  target_department: boolean
}

export interface ConnectionIcon {
  key: string
  by_sub_type?: Record<string, string>
}

export interface ConnectionSourceField {
  key: string
  label: string
  sample?: string
  group: 'account' | 'organization'
}

export interface ConnectionTypeInfo {
  key: string
  direction: ConnectionDirection
  name: string
  description: string
  vendor?: string
  icon: ConnectionIcon
  auth_kind?: string
  fields: ConnectionField[]
  variants?: ConnectionVariant[]
  capabilities: ConnectionCapabilities
  source_fields?: ConnectionSourceField[]
}

export interface ConnectionTargetField {
  key: string
  label: string
  group: 'account' | 'organization'
  /** 界面展示用的技术名（如 user.name） */
  display?: string
  required?: boolean
  hint?: string
}

export interface ConnectionCatalog {
  directions: { direction: ConnectionDirection; types: ConnectionTypeInfo[] }[]
  target_fields: ConnectionTargetField[]
}
