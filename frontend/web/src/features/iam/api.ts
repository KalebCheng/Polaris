import { api } from '@/shared/api/client'
import type {
  Application,
  ApplicationAccess,
  ApplicationAccessInput,
  ApplicationCreated,
  ApplicationSubjectOptionPage,
  AccountStrategy,
  AppAccount,
  Cert,
  Connector,
  Department,
  Group,
  Institution,
  InstitutionInput,
  PageData,
  Permission,
  Provider,
  AuditRecord,
  ProvisionHistory,
  Role,
  SessionInfo,
  StorageSettings,
  StorageSettingsInput,
  Syncer,
  SyncerRun,
  Token,
  User,
  UserAuthorizations,
  UserProfileInput,
  UserSecurity,
  GroupDetail,
  InstitutionStats,
  SyncerPreview,
  ProviderCatalog,
  UserIdentity,
  ConnectionCatalog,
} from '@/shared/api/types'
import { getActiveInstitution } from './active-institution'

/**
 * 组织内请求统一带上当前工作区组织。
 *
 * 后端组织内资源的目标组织由 `instOf(c)` 解析：优先取 `?institution=`，否则回退到登录会话所在组织。
 * 列表/查询一直带着它；但写操作（reset-password / delete / update …）曾漏带，导致平台管理员
 * 在「非会话组织」的工作区操作时，后端按会话组织找不到资源（报「用户不存在」等）。
 * 此处把当前工作区组织作为**缺省值**注入；调用方显式传入的 institution 优先。
 */
function orgQ(
  query?: Record<string, string | number | null | undefined>,
): Record<string, string | number | null | undefined> {
  const institution = getActiveInstitution()
  const merged: Record<string, string | number | null | undefined> = { ...query }
  if (!merged.institution && institution) merged.institution = institution
  return merged
}

/** 列表查询参数 */
export interface ListParams {
  institution?: string
  keyword?: string
  page?: number
  size?: number
  /** 按提供商类别过滤（provider.Api.list 使用；取值见目录的类别 key） */
  category?: string
  /** scope=platform 时跨组织列出（平台级资源：角色 / 权限 / 证书 / 令牌 / 审计） */
  scope?: 'platform'
}

function toQuery(params: ListParams = {}) {
  return {
    // 后端租户参数名为 institution，值是组织**主键 id**（字符串）；未指定时不发送，避免 NaN
    institution: params.institution,
    category: params.category,
    keyword: params.keyword,
    page: params.page ?? 1,
    size: params.size ?? 20,
    scope: params.scope,
  }
}

export const institutionApi = {
  list: (params?: ListParams) => api.get<PageData<Institution>>('/institutions', toQuery(params)),
  get: (id: number) => api.get<Institution>(`/institutions/${id}`),
  create: (body: InstitutionInput) => api.post<Institution>('/institutions', body),
  update: (id: number, body: InstitutionInput) => api.put<void>(`/institutions/${id}`, body),
  remove: (id: number) => api.del<void>(`/institutions/${id}`),
  /** 单组织资源聚合 */
  stats: (id: number) => api.get<InstitutionStats>(`/institutions/${id}/stats`),
  /** 全部组织的资源聚合（总览页按组织聚合） */
  statsAll: () => api.get<{ items: InstitutionStats[] }>('/institutions/stats'),
}

export const storageApi = {
  getSettings: () => api.get<StorageSettings>('/system/storage'),
  updateSettings: (body: StorageSettingsInput) => api.put<void>('/system/storage', body),
  testSettings: () => api.post<void>('/system/storage/test'),
}

export const departmentApi = {
  list: (params?: ListParams) => api.get<PageData<Department>>('/departments', orgQ(toQuery(params))),
  get: (id: number, institution?: string) =>
    api.get<Department>(`/departments/${id}`, orgQ({ institution })),
  create: (body: Partial<Department>) => api.post<Department>('/departments', body, orgQ()),
  update: (id: number, body: Partial<Department>) => api.put<void>(`/departments/${id}`, body, orgQ()),
  remove: (id: number) => api.del<void>(`/departments/${id}`, orgQ()),
  members: (id: number, institution?: string) =>
    api.get<{ items: User[] }>(`/departments/${id}/members`, orgQ({ institution })),
  /** 部门成员整体替换（列表中的人挂到本部门，未列出的移出） */
  setMembers: (id: number, users: string[]) => api.put<void>(`/departments/${id}/members`, { users }, orgQ()),
  reorder: (id: number, direction: 'up' | 'down') =>
    api.post<void>(`/departments/${id}/reorder`, { direction }, orgQ()),
}

/** 用户目录筛选（后端 /users 的查询参数） */
export interface UserListParams extends ListParams {
  department_id?: number
  /** active | disabled | deleted */
  status?: string
  mfa?: boolean
}

export const userApi = {
  list: (params?: UserListParams) =>
    api.get<PageData<User>>(
      '/users',
      orgQ({
        ...toQuery(params),
        department_id: params?.department_id,
        status: params?.status,
        // 布尔在查询串里只能以字符串出现（后端按 strconv.ParseBool 解析）
        mfa: params?.mfa === undefined ? undefined : String(params.mfa),
      }),
    ),
  get: (id: number, institution?: string) => api.get<User>(`/users/${id}`, orgQ({ institution })),
  // 创建入参与编辑入参共享同一套资料字段（后端有反射测试锁定两侧一致，
  // 防止"表单能填、后端丢弃"——第四阶段同类事故）
  create: (body: { name: string; department_id?: number | null; is_admin?: boolean; password?: string } & UserProfileInput) =>
    api.post<User>('/users', body, orgQ()),
  // 资料部分更新：只提交需要变更的字段（未提交的字段一律保持原值）
  updateProfile: (id: number, body: UserProfileInput) => api.put<User>(`/users/${id}`, body, orgQ()),
  setDepartment: (id: number, departmentId: number | null) =>
    api.put<void>(`/users/${id}/department`, { department_id: departmentId }, orgQ()),
  setAdmin: (id: number, isAdmin: boolean) => api.put<void>(`/users/${id}/admin`, { is_admin: isAdmin }, orgQ()),
  setDisabled: (id: number, disabled: boolean) =>
    api.post<void>(`/users/${id}/${disabled ? 'disable' : 'enable'}`, {}, orgQ()),
  remove: (id: number) => api.del<void>(`/users/${id}`, orgQ()),
  resetPassword: (id: number, newPassword: string) =>
    api.post<void>(`/users/${id}/reset-password`, { new_password: newPassword }, orgQ()),
  /** 重置 MFA（清空绑定；需 user:reset_mfa 权限） */
  resetMfa: (id: number) => api.post<void>(`/users/${id}/reset-mfa`, {}, orgQ()),
  /** 解除登录失败锁定（需 user:unlock 权限） */
  unlock: (id: number) => api.post<{ cleared: number }>(`/users/${id}/unlock`, {}, orgQ()),
  security: (id: number) => api.get<UserSecurity>(`/users/${id}/security`, orgQ()),
  authorizations: (id: number) => api.get<UserAuthorizations>(`/users/${id}/authorizations`, orgQ()),
  setRoles: (id: number, roles: string[]) => api.put<void>(`/users/${id}/roles`, { roles }, orgQ()),
  setGroups: (id: number, groups: string[]) => api.put<void>(`/users/${id}/groups`, { groups }, orgQ()),
  appAccounts: (id: number) => api.get<{ items: AppAccount[] }>(`/users/${id}/app-accounts`, orgQ()),
  provisionHistory: (id: number, params?: ListParams) =>
    api.get<PageData<ProvisionHistory>>(`/users/${id}/provision-history`, orgQ(toQuery(params))),
  records: (id: number, params?: ListParams) =>
    api.get<PageData<AuditRecord>>(`/users/${id}/records`, orgQ(toQuery(params))),
  identities: (id: number) => api.get<{ items: UserIdentity[] }>(`/users/${id}/identities`, orgQ()),
  unbindIdentity: (id: number, identityId: number) =>
    api.del<void>(`/users/${id}/identities/${identityId}`, orgQ()),
  issueApiKey: (id: number, body: { name?: string; expire_in_hours?: number }) =>
    api.post<{ name: string; token: string; expires_at: string | null }>(
      `/users/${id}/api-keys`,
      body,
      orgQ(),
    ),
}

export const applicationApi = {
  list: (params?: ListParams) => api.get<PageData<Application>>('/applications', orgQ(toQuery(params))),
  /** 访问授权（谁能使用该应用）：访问模式 + 三类主体名单 */
  access: (id: number, institution?: string) =>
    api.get<ApplicationAccess>(`/applications/${id}/access`, orgQ({ institution })),
  /** 整体替换访问授权（空名单允许保存 = 该应用暂时无人可访问） */
  setAccess: (id: number, body: ApplicationAccessInput) =>
    api.put<ApplicationAccess>(`/applications/${id}/access`, body, orgQ()),
  /**
   * 可授权主体候选（按组织；新建应用时还没有主键也能取）。
   * 独立端点而非复用 /users、/groups、/departments：应用管理员通常没有那些读权限，
   * 复用会让选择器空转——"配了但选不出人"是最难排查的死配置。
   *
   * 分页：offset/limit + total（扁平列表分页器）。
   * 部门树：不传 keyword 时按层返回**直接子节点**，`parentId` 指定要展开的节点（省略 = 顶层）。
   */
  subjectOptions: (params: {
    type: string
    keyword?: string
    parentId?: number
    offset?: number
    limit?: number
  }) =>
    api.get<ApplicationSubjectOptionPage>('/applications/subject-options', {
      type: params.type,
      keyword: params.keyword,
      parent_id: params.parentId,
      offset: params.offset,
      limit: params.limit,
      ...orgQ(),
    }),
  /** code 为应用标识（创建后不可修改） */
  get: (id: number, institution?: string) =>
    api.get<Application>(`/applications/${id}`, orgQ({ institution })),
  /** 创建：响应会一次性返回明文 client_secret 与端点清单 */
  create: (body: Partial<Application>) => api.post<ApplicationCreated>('/applications', body, orgQ()),
  update: (id: number, body: Partial<Application>) => api.put<void>(`/applications/${id}`, body, orgQ()),
  remove: (id: number) => api.del<void>(`/applications/${id}`, orgQ()),
  /** 重置密钥：同样一次性返回明文 */
  resetSecret: (id: number) =>
    api.post<{ client_secret: string; has_client_secret: boolean }>(
      `/applications/${id}/reset-secret`,
      undefined,
      orgQ(),
    ),
}

export interface ProviderPayload {
  institution_id?: number
  name?: string
  display_name?: string
  category?: string
  type?: string
  state?: string
  /** 非敏感配置（键由类型目录声明；未知键会被后端拒绝） */
  config?: Record<string, unknown>
  /** 敏感明文（键由类型目录声明；留空表示保持原值） */
  secrets?: Record<string, string>
  /** SSO 信任策略（仅外部身份提供方） */
  policy?: Record<string, unknown>
}

export const providerApi = {
  list: (params?: ListParams) => api.get<PageData<Provider>>('/providers', orgQ(toQuery(params))),
  get: (id: number, institution?: string) => api.get<Provider>(`/providers/${id}`, orgQ({ institution })),
  /** 提供商目录（类别 / 类型 / 字段 schema）：目录页与向导的唯一来源 */
  catalog: () => api.get<ProviderCatalog>('/providers/catalog'),
  create: (body: ProviderPayload) => api.post<Provider>('/providers', body, orgQ()),
  update: (id: number, body: ProviderPayload) => api.put<Provider>(`/providers/${id}`, body, orgQ()),
  remove: (id: number) => api.del<void>(`/providers/${id}`, orgQ()),
  /** 连通性自检（邮件/短信/人机校验/存储/外部 IdP 各有对应检查） */
  test: (id: number) => api.post<{ status: string }>(`/providers/${id}/test`, undefined, orgQ()),
  /**
   * 只切换启用状态（列表卡片快捷开关）。
   * 单例类别（邮件/短信/人机校验/存储）同一组织最多一个启用项，切换冲突时后端返回 409。
   */
  setState: (id: number, state: 'active' | 'disabled') =>
    api.post<Provider>(`/providers/${id}/state`, { state }, orgQ()),
}

export interface RoleRelations {
  users?: string[]
  groups?: string[]
  roles?: string[]
  permissions?: string[]
}

// 平台级资源（角色/权限/证书/令牌）：列表用 scope=platform 跨组织，
// 但改名/删除等「按名字定位」的操作必须在资源所属组织内执行——传入资源的 institution。
export const roleApi = {
  list: (params?: ListParams) => api.get<PageData<Role>>('/roles', toQuery(params)),
  get: (id: number, institution?: string) => api.get<Role>(`/roles/${id}`, { institution }),
  create: (body: Partial<Role>) => api.post<Role>('/roles', body),
  update: (id: number, body: Partial<Role>, institution?: string) =>
    api.put<void>(`/roles/${id}`, body, { institution }),
  setRelations: (id: number, body: RoleRelations, institution?: string) =>
    api.put<void>(`/roles/${id}/relations`, body, { institution }),
  remove: (id: number, institution?: string) => api.del<void>(`/roles/${id}`, { institution }),
}

export interface PermissionSubjects {
  users?: string[]
  groups?: string[]
  roles?: string[]
}

export const permissionApi = {
  list: (params?: ListParams) => api.get<PageData<Permission>>('/permissions', toQuery(params)),
  get: (id: number, institution?: string) => api.get<Permission>(`/permissions/${id}`, { institution }),
  create: (body: Partial<Permission>) => api.post<Permission>('/permissions', body),
  update: (id: number, body: Partial<Permission>, institution?: string) =>
    api.put<void>(`/permissions/${id}`, body, { institution }),
  setSubjects: (id: number, body: PermissionSubjects, institution?: string) =>
    api.put<void>(`/permissions/${id}/subjects`, body, { institution }),
  remove: (id: number, institution?: string) => api.del<void>(`/permissions/${id}`, { institution }),
  enforce: (body: { institution?: string; user: string; resource: string; action: string }) =>
    api.post<{ allowed: boolean; roles: string[] | null }>('/permissions/enforce', body),
}

export const groupApi = {
  list: (params?: ListParams) => api.get<PageData<Group>>('/groups', orgQ(toQuery(params))),
  get: (id: number, institution?: string) => api.get<Group>(`/groups/${id}`, orgQ({ institution })),
  create: (body: Partial<Group>) => api.post<Group>('/groups', body, orgQ()),
  update: (id: number, body: Partial<Group>) => api.put<void>(`/groups/${id}`, body, orgQ()),
  setMembers: (id: number, users: string[]) => api.put<void>(`/groups/${id}/members`, { users }, orgQ()),
  /** 组详情：成员明细 + 关联角色 + 汇总权限 */
  detail: (id: number, institution?: string) => api.get<GroupDetail>(`/groups/${id}/detail`, orgQ({ institution })),
  remove: (id: number) => api.del<void>(`/groups/${id}`, orgQ()),
}

export const certApi = {
  list: (params?: ListParams) => api.get<PageData<Cert>>('/certs', toQuery(params)),
  get: (id: number, institution?: string) => api.get<Cert>(`/certs/${id}`, { institution }),
  create: (body: Partial<Cert>) => api.post<Cert>('/certs', body),
  update: (id: number, body: Partial<Cert>, institution?: string) =>
    api.put<void>(`/certs/${id}`, body, { institution }),
  remove: (id: number, institution?: string) => api.del<void>(`/certs/${id}`, { institution }),
}

export const tokenApi = {
  list: (params?: ListParams) => api.get<PageData<Token>>('/tokens', toQuery(params)),
  get: (id: number, institution?: string) => api.get<Token>(`/tokens/${id}`, { institution }),
  revoke: (id: number, institution?: string) =>
    api.post<void>(`/tokens/${id}/revoke`, undefined, { institution }),
  remove: (id: number, institution?: string) => api.del<void>(`/tokens/${id}`, { institution }),
}

export const sessionApi = {
  listByUser: (user: string, institution?: string) =>
    api.get<{ items: SessionInfo[] }>('/sessions', { user, institution }),
  /** 平台级：跨组织列出全部活动会话 */
  listAll: () => api.get<{ items: SessionInfo[] }>('/sessions', { scope: 'platform' }),
  revoke: (sessionId: string, user: string) =>
    api.del<void>(`/sessions/${sessionId}`, { user, scope: 'platform' }),
  revokeAll: (user: string) => api.post<void>('/sessions/revoke-all', { user }),
}

export const recordApi = {
  list: (params?: ListParams) => api.get<PageData<AuditRecord>>('/records', toQuery(params)),
  get: (id: number) => api.get<AuditRecord>(`/records/${id}`),
}

export interface SyncerPayload {
  institution?: string
  name: string
  display_name?: string
  type: string
  is_enabled?: boolean
  schedule?: string
  host?: string
  port?: number
  user?: string
  database?: string
  table?: string
  database_type?: string
  app_id?: string
  corp_id?: string
  sync_users?: boolean
  sync_groups?: boolean
  soft_delete?: boolean
  /** 明文口令（后端加密落库；留空表示不修改） */
  secret?: string
  /** 同步目标：外部组织与账号挂到该部门节点之下（null = 组织根） */
  target_department_id?: number | null
  /** 同步配置：定时校验与同步保护（与界面一一对应） */
  sync_config?: {
    incremental?: boolean
    schedule?: {
      mode?: 'interval' | 'daily' | 'cron' | ''
      interval_seconds?: number
      times?: string[]
      cron?: string
      window_minutes?: number
    }
    guard?: { max_user_deletes?: number }
  }
  /** 字段映射：本地字段 → 源字段（仅声明支持映射的类型生效） */
  user_mapping?: Record<string, string>
  /** 明文应用密钥（后端加密落库；留空表示不修改） */
  app_secret_plain?: string
}

export const syncerApi = {
  types: () => api.get<{ types: string[] }>('/syncers/types'),
  list: (params?: ListParams) => api.get<PageData<Syncer>>('/syncers', orgQ(toQuery(params))),
  get: (id: number, institution?: string) => api.get<Syncer>(`/syncers/${id}`, orgQ({ institution })),
  create: (body: SyncerPayload) => api.post<Syncer>('/syncers', body, orgQ()),
  update: (id: number, body: SyncerPayload) => api.put<void>(`/syncers/${id}`, body, orgQ()),
  remove: (id: number) => api.del<void>(`/syncers/${id}`, orgQ()),
  /**
   * 触发同步：**入队后立即返回**（202 + 任务 id），执行由 asyncd 的 worker 完成。
   * `status === 'running'` 表示该数据源已有任务在跑（单飞，不会并发跑两次）。
   */
  run: (id: number) => api.post<{ task_id: number; status: 'queued' | 'running' }>(
    `/syncers/${id}/run`,
    undefined,
    orgQ(),
  ),
  /** 试运行：拉取 + 差异 + 同步保护校验，不写任何数据 */
  dryRun: (id: number) => api.post<SyncerPreview>('/syncers/' + id + '/dry-run', undefined, orgQ()),
  runs: (id: number, params?: { page?: number; size?: number }) =>
    api.get<{ items: SyncerRun[] }>(
      `/syncers/${id}/runs`,
      orgQ({
        page: params?.page ?? 1,
        size: params?.size ?? 20,
      }),
    ),
}

export interface ConnectorPayload {
  institution?: string
  name: string
  display_name?: string
  application?: string
  /** 连接器类型（缺省 http） */
  type?: string
  just_in_time?: boolean
  state?: string
  /** 类型专属的非敏感配置（键必须由类型目录声明） */
  config?: Record<string, string>
  /** 明文敏感值（键为目录中声明为 secret 的字段；留空表示不修改） */
  secrets?: Record<string, string>
}

export const connectorApi = {
  types: () => api.get<{ types: string[] }>('/connectors/types'),
  list: (params?: ListParams) => api.get<PageData<Connector>>('/connectors', orgQ(toQuery(params))),
  get: (id: number, institution?: string) =>
    api.get<Connector>(`/connectors/${id}`, orgQ({ institution })),
  create: (body: ConnectorPayload) => api.post<Connector>('/connectors', body, orgQ()),
  update: (id: number, body: ConnectorPayload) => api.put<void>(`/connectors/${id}`, body, orgQ()),
  remove: (id: number) => api.del<void>(`/connectors/${id}`, orgQ()),
  test: (id: number) => api.post<{ status: string }>(`/connectors/${id}/test`, undefined, orgQ()),
  /** 对齐检查：核对本地应用账号在下游是否真实存在（默认只报告，不入队） */
  reconcile: (id: number, dryRun = true) =>
    api.post<ReconcileReport>(`/connectors/${id}/reconcile`, { dry_run: dryRun }, orgQ()),
}

export const accountStrategyApi = {
  list: (params?: ListParams) =>
    api.get<PageData<AccountStrategy>>('/account-strategies', orgQ(toQuery(params))),
  get: (id: number, institution?: string) =>
    api.get<AccountStrategy>(`/account-strategies/${id}`, orgQ({ institution })),
  create: (body: Partial<AccountStrategy>) => api.post<AccountStrategy>('/account-strategies', body, orgQ()),
  update: (id: number, body: Partial<AccountStrategy>) =>
    api.put<void>(`/account-strategies/${id}`, body, orgQ()),
  remove: (id: number) => api.del<void>(`/account-strategies/${id}`, orgQ()),
}

export const appAccountApi = {
  list: (params?: ListParams) => api.get<PageData<AppAccount>>('/app-accounts', orgQ(toQuery(params))),
  create: (body: { institution?: string; application: string; user: string }) =>
    api.post<void>('/app-accounts', body, orgQ()),
  retry: (body: { institution?: string; application: string; user: string }) =>
    api.post<void>('/app-accounts/retry', body, orgQ()),
  remove: (user: string, application: string, institution?: string) =>
    api.del<void>('/app-accounts', orgQ({ user, application, institution })),
  /** 门户：当前用户自己的应用账号（只读） */
  mine: (institution?: string) => api.get<{ items: AppAccount[] }>('/me/app-accounts', { institution }),
}

export const provisionHistoryApi = {
  list: (params?: ListParams) => api.get<PageData<ProvisionHistory>>('/provisioning', orgQ(toQuery(params))),
}

/**
 * 身份连接：类型目录（后端是类型/字段 schema 的单一来源，见 docs/specs/261013-identity-connections-phase3.md FR-2）。
 * 入方向与出方向共用同一个目录端点。
 */
export const connectionApi = {
  catalog: () => api.get<ConnectionCatalog>('/connections/catalog'),
}

/** 平台级 OAuth 接入信息：提供商向导（外部身份提供方的「高级与策略」步）展示，供回填到 IdP 后台 */
export const platformApi = {
  oauthInfo: () => api.get<{ callback_url: string; homepage_url: string }>('/system/oauth'),
}

/** 对齐检查结果条目：ok=下游确认存在，missing=确认缺失，unknown=无法判定（不可当作缺失） */
export interface ReconcileItem {
  user: string
  status: 'ok' | 'missing' | 'unknown'
  detail?: string
}

export interface ReconcileReport {
  application: string
  connector: string
  dry_run: boolean
  total: number
  ok: number
  missing: number
  unknown: number
  queued: number
  truncated: boolean
  items: ReconcileItem[]
}

/** 定时任务的调度规则（异步任务没有这一段） */
export interface AsyncdSchedule {
  kind: string
  interval_seconds: number
  daily_times: string[]
  window_minutes: number
  enabled: boolean
  next_run_at: string
  last_run_at?: string | null
  last_status: string
}

/** 一个任务的执行统计（来自 asyncd_job_runs 的聚合） */
export interface AsyncdJobStats {
  total: number
  by_status: Record<string, number>
  last_queued_at?: string | null
}

/**
 * 任务定义（asyncd_jobs）：任务中心里的一个「任务」。
 * - `kind === 'scheduled'`：定时任务，带 `schedule` 规则与启停；
 * - `kind === 'async'`：异步任务，由业务动作按需入队。
 * 执行记录用 `id`（job_id）去查。
 */
export interface AsyncdJob {
  id: number
  name: string
  task_type: string
  kind: 'async' | 'scheduled'
  schedule?: AsyncdSchedule
  stats: AsyncdJobStats
}

/** 执行实例（asyncd_job_runs）：一行 = 一次执行 */
export interface AsyncdJobRun {
  id: number
  job_id: number
  job_name: string
  task_type: string
  /** queued | running | succeeded | dead */
  status: string
  priority: number
  attempts: number
  max_attempts: number
  next_attempt_at: string
  unique_key?: string
  worker_id?: string
  last_error?: string
  institution_id?: number
  created_at: string
  updated_at: string
  finished_at?: string | null
}

/** 执行记录（asyncd_job_run_attempts）：一行 = 一次执行里的一次尝试 */
export interface AsyncdJobRunAttempt {
  id: number
  attempt_no: number
  worker_id: string
  /** running | succeeded | failed | lease_lost */
  status: string
  started_at: string
  finished_at?: string | null
  duration_ms: number
  error?: string
}

/** 执行器实例（asyncd_workers）：一个可领取任务的槽位；state 由心跳算出 */
export interface AsyncdWorker {
  id: string
  instance_id: string
  host: string
  pid: number
  slot_index: number
  /** online | stale | stopped */
  state: string
  started_at: string
  last_heartbeat_at: string
  stopped_at?: string | null
  running: number
}

/**
 * asyncd：任务定义 / 执行实例 / 执行记录 / 执行器实例（平台级）。
 * 运维靠它回答四个问题：有哪些任务、现在队列里怎么样、某次执行当时发生了什么、谁在跑。
 */
export const asyncdApi = {
  /** 任务目录：全部任务定义（异步 + 定时）及执行统计 */
  jobs: () => api.get<{ items: AsyncdJob[] }>('/asyncd/jobs'),
  /** 启停定时任务（异步任务不提供停用） */
  setJobEnabled: (id: number, enabled: boolean) => api.put<void>(`/asyncd/jobs/${id}`, { enabled }),
  /**
   * 某任务的执行记录（执行实例）：路径嵌套在任务定义下（执行实例必然属于某个任务定义）。
   * `counts` 与列表同一作用域，前端据此派生分页总数。
   */
  jobRuns: (jobId: number, params?: { status?: string; page?: number; size?: number }) =>
    api.get<{ items: AsyncdJobRun[]; counts: Record<string, number>; page: number; size: number }>(
      `/asyncd/jobs/${jobId}/runs`,
      { status: params?.status, page: params?.page ?? 1, size: params?.size ?? 20 },
    ),
  /** 某次执行的尝试明细（执行记录） */
  jobRunAttempts: (jobId: number, runId: number) =>
    api.get<{ items: AsyncdJobRunAttempt[] }>(`/asyncd/jobs/${jobId}/runs/${runId}/attempts`),
  /** 人工重试死信 */
  retryJobRun: (jobId: number, runId: number) =>
    api.post<void>(`/asyncd/jobs/${jobId}/runs/${runId}/retry`, undefined),
  workers: () => api.get<{ items: AsyncdWorker[] }>('/asyncd/workers'),
}
