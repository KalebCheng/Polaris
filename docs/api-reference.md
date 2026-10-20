# API 参考

> Polaris 管理 API 的接口清单与约定。协议端点（OIDC）见《OIDC 接入指南》。

---

## 1. 总览

- **基址**：`/api/v1`
- **内容类型**：`application/json`（协议端点 `/oauth/*` 使用表单编码）
- **认证**：登录后由 HttpOnly Cookie 承载会话 JWT；也支持 `Authorization: Bearer <access_token>`（OAuth 令牌）
- **响应信封**：

```json
{ "code": "OK",       "message": "ok",                 "data": { ... } }
{ "code": "NOT_FOUND","message": "resource not found", "details": { ... } }
```

成功为 `200`（创建为 `201`）；错误由统一错误码表达（见 §4）。
写操作为 Cookie 环境凭证时需 CSRF：`Origin` 必须命中 `security.csrf_allowed_origins`；显式携带 `Authorization` 的请求跳过 CSRF。

多租户：多数资源路由以 `institution` 作为归属维度；`institutions` 路由以 `:name` 指定租户；部分列表接口可用 `?institution=` 过滤。

---

## 2. 公开端点（无需登录）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz` | 存活探针 |
| GET | `/readyz` | 就绪探针（含依赖状态） |
| GET | `/.well-known/openid-configuration` | OIDC 发现文档 |
| GET | `/.well-known/jwks` | JWKS |
| GET | `/oauth/authorize` | 授权端点（实际要求登录态） |
| POST | `/oauth/token` | 令牌端点 |
| GET/POST | `/oauth/userinfo` | 用户信息 |
| POST | `/oauth/introspect` | 令牌内省 |
| GET/POST | `/oauth/logout` | 结束会话 |
| GET | `/static/storage/*path` | 本地存储直出 |

认证（注册/登录，公开）：

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/auth/login` | 密码登录 |
| POST | `/api/v1/auth/signup` | 注册 |
| GET | `/api/v1/auth/status` | 会话状态 |
| GET | `/api/v1/auth/providers` | 可用登录提供方（读提供商表，改完即生效） |
| GET | `/api/v1/auth/institutions` | 可登录租户 |
| GET | `/api/v1/auth/app-info` | 登录页公开应用信息 |
| GET | `/api/v1/auth/captcha` | 获取验证码 |
| POST | `/api/v1/auth/codes` | 发送验证码 |
| POST | `/api/v1/auth/login/code` | 验证码登录 |
| GET/POST | `/api/v1/auth/oauth/:provider/callback` | 外部 IdP 回调（state 内含组织，回到同一提供商） |

---

## 3. 需要认证的管理端点

### 3.1 当前用户（自助）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/auth/logout` | 登出（触发 back-channel logout） |
| GET | `/me` | 当前用户资料 |
| POST | `/me/avatar` | 上传头像 |
| GET | `/me/identities` | 我的外部身份 |
| DELETE | `/me/identities/:identityID` | 解绑身份 |
| POST | `/me/identities/:provider/authorize` | 发起身份绑定 |
| GET | `/me/sessions` | 我的会话 |
| DELETE | `/me/sessions/:sessionID` | 撤销我的会话 |
| POST | `/me/sessions/revoke-all` | 撤销全部我的会话 |
| GET | `/me/records` | 我的审计记录 |
| GET | `/auth/mfa/options` | pending 会话的 MFA 选项与已绑定凭证 |
| POST | `/auth/mfa/enroll` · `/enroll/verify` | 绑定 TOTP / 短信 MFA |
| POST | `/auth/mfa/verify` · `/skip` · `/disable` | 校验、可选跳过、关闭 MFA |
| POST | `/auth/change-password` | 改密；账号尚无密码（如第三方登录自动建号）时可省略 `old_password`，直接完成首次设密 |
| GET | `/me/applications` | 我的应用中心应用（含 `client_id` / `redirect_uri`，供一键 SSO） |
| GET | `/me/application-accounts` | 我的应用账号（只读） |

### 3.2 租户与部门

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/institutions` | 认证 | 租户列表 |
| POST | `/institutions` | admin | 创建租户 |
| GET | `/institutions/:name` | 认证 | 租户详情 |
| GET | `/institutions/:name/stats` | 认证 | 租户资源统计 |
| GET | `/institutions/stats` | 认证 | 全部租户统计 |
| PUT | `/institutions/:name` | admin | 更新租户 |
| DELETE | `/institutions/:name` | admin | 删除租户 |
| GET/POST | `/departments` | `department:read/create` | 部门列表/创建 |
| GET/PUT/DELETE | `/departments/:name` | `department:read/update/delete` | 部门详情/更新/删除 |
| GET | `/departments/:name/members` | `department:read` | 部门成员（归属该部门的用户） |
| POST | `/departments/:name/reorder` | `department:update` | 同级上/下移动（body `{"direction":"up\|down"}`） |

### 3.3 用户 / 组 / 角色 / 权限

| 方法 | 路径 | 权限 |
|------|------|------|
| GET/POST | `/users` | `user:read` / `user:create` |
| GET/PUT/DELETE | `/users/:name` | `user:read/update/delete` |
| POST | `/users/:name/reset-password` | `user:update` |
| GET | `/users/:name/identities` | `user:read` |
| DELETE | `/users/:name/identities/:identityID` | `user:update` |
| POST | `/users/:name/api-keys` | `user:update` |
| GET/POST | `/job-titles` · GET/PUT/DELETE `/job-titles/:id` | `job_title:*` |
| PUT | `/job-titles/:id/members` | `job_title:update` |
| GET | `/job-titles/:id/detail` | `job_title:read` |
| GET/POST | `/job-levels` · GET/PUT/DELETE `/job-levels/:id` | `job_level:*` |
| GET | `/job-levels/distribution` | `job_level:read` |
| PUT | `/users/:id/departments` | `user:update` |
| PUT | `/users/:id/job-level` | `user:update` |
| PUT | `/users/:id/job-titles` | `user:update` |
| GET/POST | `/roles` · GET/PUT/DELETE `/roles/:name` | `role:*` |
| PUT | `/roles/:name/relations` | `role:update` |
| GET/POST | `/permissions` · GET/PUT/DELETE `/permissions/:name` | `permission:*` |
| PUT | `/permissions/:name/subjects` | `permission:update` |
| POST | `/permissions/enforce` | `permission:read` |

### 3.4 应用 / 提供方 / 证书 / 令牌

| 方法 | 路径 | 权限 |
|------|------|------|
| GET/POST | `/applications` | `application:read/create` |
| GET/PUT/DELETE | `/applications/:name` | `application:read/update/delete` |
| POST | `/applications/:name/reset-secret` | `application:update` |
| GET/PUT | `/applications/:name/access` | `application:read/update` |
| GET | `/applications/subject-options?type=user\|job_title\|department&keyword=&parent_id=&offset=&limit=` | `application:read` |
| GET/POST | `/providers` · GET/PUT/DELETE `/providers/:id` | `provider:*` |
| GET | `/providers/catalog` · POST `/providers/:id/test` | `provider:read/update` |
| GET/POST | `/providers/:id/message-templates` | `provider:read/update` |
| GET/PUT/DELETE | `/providers/:id/message-templates/:templateId` | `provider:read/update/delete` |
| GET/POST | `/certs` · GET/PUT/DELETE `/certs/:name` | `cert:*` |
| GET | `/tokens` · GET `/tokens/:name` | `token:read` |
| POST | `/tokens/:name/revoke` · DELETE `/tokens/:name` | `token:update/delete` |

**应用字段补充（P4）**：`adapter`（`default`/`custom`）与 `adapter_config`（`{"claimMap": {...}}`），见《OIDC 接入指南》§4.1。

用户、职务与部门成员关系接口统一使用主键数组：`PUT /users/:id/roles` 提交 `{"role_ids":[...]}`，`PUT /users/:id/job-titles` 提交 `{"job_title_ids":[...]}`，`PUT /job-titles/:id/members` 与 `PUT /departments/:id/members` 提交 `{"user_ids":[...]}`。名称字段只用于响应展示。

外部身份提供方的 SSO 信任策略按 `match_type` 分流：`system` 使用 `match_local_field`，`identity_link` 必须同时提供本组织的入方向连接 `match_syncer_id` 与该连接的源字段 `match_syncer_field`（`external_id` 或连接器声明的任意源字段，如 `unionid`）；两种模式都必须提供 `match_provider_field`。

`identity_link` 表达"提供商与这条连接是同一个外部系统"，构成**双向认领**：登录时按 `match_provider_field` 的值在连接的 `syncer_user_links` 里匹配；同步时反过来按连接源字段的值在 `user_identities.attributes` 里匹配，找回此前的 SSO 建号用户——于是"先登录后同步"与"先同步后登录"收敛到同一本地用户。匹配只查询绑定的这条连接，不会跨连接模糊匹配。

自动建号（`auto_create`）的字段来源由 `provision_username_field` / `provision_name_field` / `provision_email_field` / `provision_phone_field` / `provision_avatar_field` 声明（留空用默认字段：用户名、昵称、email、mobile、avatar）；建号时同时写入 `user_identities.attributes`（提供商侧原始字段），供上述反向认领使用。

**应用访问授权（第六阶段）**：`applications.access_mode` 为 `all`（全员可访问，默认）或 `restricted`（仅授权名单内主体）。

- `PUT /applications/:name/access`：整体替换访问模式与名单，请求体 `{"access_mode":"restricted","user_ids":[101],"job_title_ids":[201],"department_ids":[301]}`；
  **空名单是合法输入**（= 该应用暂时无人可访问，可保存后补授权）；不存在的主体一律 `INVALID_PARAMS`（不做静默忽略）。
- 语义：部门含子部门（"授权父级 = 覆盖整个子树"）；**职务是扁平的**——授权某职务即覆盖其全部任职人员，不再有"子职务"概念（Spec 第六阶段 Q4）。判定**对所有人一致，没有管理员直通**。
- 判定作用域：只在请求携带**应用上下文**（`client_id` / `application`）时生效——应用的登录、授权码、令牌（含 `refresh_token` 轮换）与门户应用中心都被约束，
  而"不带应用上下文的登录"= 登录平台，**不做准入判定**（因此误配永远不会把人挡在控制台之外）。
- 拒绝表现：登录族返回 `403 APP_ACCESS_DENIED` 且不建立会话；`/oauth/authorize` 按 OAuth2 规范回跳 `redirect_uri?error=access_denied&state=...`。
- `GET /applications/:name/access` 返回 `{access_mode, users[], groups[], departments[]}`，每项含 `id` / `name` / `display_name` / `path` / `missing`；`id` 是主体主键，`name` 只用于展示。
- 应用列表额外下发 `access_mode` 与 `access_subject_count`（供列表「访问范围」列）。

**可授权主体候选（`GET /applications/subject-options`）**：按**组织**取（新建应用时还没有主键也能用），只需 `application:read`。

- 返回 `{items: [...], total: N}`；扁平列表用 `offset`/`limit` 分页（`limit` 上限 200）。
- `type=user|job_title`：扁平分页列表（**职务已扁平化**，无层级与父链），`keyword` 同时匹配标识与展示名。
  ⚠️ 响应体里职务主体的 JSON 键仍是 **`groups`**（历史键名，仅存储层改名），请求参数 `type` 用 `job_title`。
- `type=department`：
  - **带 `keyword`** → 跨层级的扁平搜索结果（带 `name_path`，便于按名字直达），与扁平列表一样分页；
  - **不带 `keyword`** → **按层返回直接子节点**（省略 `parent_id` 即顶层），每项带 `id`（展开时作为 `parent_id` 再取一层）与 `has_child`（决定是否画展开箭头）。控制台弹窗的部门树据此逐层懒加载，不会一次拉全整棵树。
- `parent_id` **只对部门有意义**：传给其它类型、或与 `keyword` 同时出现，一律 `INVALID_PARAMS`（不静默忽略）。

#### 主键关联（第六阶段 Q5：存储与写入契约只认主键）

关联列一律 `<对象>_id` + 外键；**业务键（code / name）只用于响应展示**，写入统一提交主键。
已完成：`tokens`（应用/用户）、供给域四表（`connectors` /
`account_strategies` / `application_accounts` / `provision_history`）、`job_title_members`、
`password_history`；审计 `records` 为**例外**——`user` 是事件当时的名称快照，另加可空 `user_id`
（`ON DELETE SET NULL`），删除用户不会连带删除审计行。

#### 职务 / 职级（第六阶段）

- **职务 `job_titles`**（原「用户组」）：组织架构里的「人 × 职务」关系，是批量授权的载体——可挂角色（`role_job_titles`）、
  可挂权限（`permission_job_titles`）、可作应用访问主体（`application_subjects.subject_type = 'job_title'`）。
  **扁平无层级**：飞书职务字典无层级、钉钉 `title` / 企微 `position` 是自由文本，层级无源数据可填（且旧组层级在界面上无从设置）。
  `manager` / `contact_email` 仅为展示信息，不参与任何判定。
- **职级 `job_levels`**：用户属性（`users.job_level_id` 单值引用，按 `sort_order` 升序），**不参与授权**。
  用途固定三处：用户表单/详情下拉、用户列表筛选与排序、组织概览职级分布。维护方式与职务同构（本地 CRUD + 身份源同步，**只拉不推**：
  有映射行的职级以源端为准，无映射行的本地职级同步绝不修改）。
- **外部身份映射（入方向）**：`syncer_user_links` / `syncer_department_links` / `syncer_job_title_links` / `syncer_job_level_links`，
  键为 **(连接, 源端 ID)**；同步据此判定"新增 / 更新"，源端改名不再产生新对象。`raw` 存源端报文用于排障。
- **多部门归属**：`user_departments` 是多值关系与主部门（`is_primary`）的唯一真相；
  `users.department_id` 已移除（`PUT /users/:id/departments` 的 `primary_department_id` 省略时取第一个）。
- **组织根部门**：每个组织恰好一行 `is_root = TRUE` 的部门（`GET /departments` 的 `is_root` 字段），
  顶层部门一律挂在它之下；它不可改名 / 移动 / 删除（`PROTECTED_OBJECT`）。外部系统的根占位节点（钉钉/企微 `1`、飞书 `0`）**不落库**。

### 3.5 会话 / 审计 / 同步器

| 方法 | 路径 | 权限 |
|------|------|------|
| GET | `/sessions` · DELETE `/sessions/:sessionID` · POST `/sessions/revoke-all` | `session:read/delete` |
| GET | `/records` · GET `/records/:id` | `record:read` |
| GET | `/syncers` · POST `/syncers` · GET/PUT/DELETE `/syncers/:name` | `syncer:*` |
| GET | `/syncers/types` · GET `/syncers/:name/runs` | `syncer:read` |
| POST | `/syncers/:name/run` | `syncer:update` |
| POST | `/syncers/:name/dry-run` · POST `/syncers/:name/test` | `syncer:update` |

> `POST /syncers/:id/run` 为**异步触发**：入队后立即返回 `202 {"task_id":n,"status":"queued"|"running"}`，
> 真正的执行由 asyncd 的 worker 完成（同一数据源同时只允许一个任务，`status=running` 表示复用了在跑的任务）。
> `dry-run` 返回"将创建/更新/停用/组织变更"的数量与同步保护是否触发，**不写任何数据**。

### 3.7 身份连接（目录 / 类型元数据）

| 方法 | 路径 | 权限 |
|------|------|------|
| GET | `/connections/catalog` | `syncer:read` 或 `connector:read` |

> 返回入/出方向的**类型目录**：类型、字段 schema（含归属步骤与落库路径 `store`）、能力声明、
> 可映射源字段与目标字段。前端据此渲染目录卡片与向导表单（类型与字段的单一来源）。

### 3.8 asyncd（任务定义 / 执行实例 / 执行记录 / 执行器实例）

| 方法 | 路径 | 权限 |
|------|------|------|
| GET | `/asyncd/jobs` | 平台管理员 |
| PUT | `/asyncd/jobs/:id` | 平台管理员 |
| GET | `/asyncd/jobs/:id/runs?status=&page=&size=` | 平台管理员 |
| GET | `/asyncd/jobs/:id/runs/:runId/attempts` | 平台管理员 |
| POST | `/asyncd/jobs/:id/runs/:runId/retry` | 平台管理员 |
| GET | `/asyncd/workers` | 平台管理员 |

> **四个维度**（表结构见 `docs/specs/261015-asyncd-task-model.md` §4）：
> 1. **任务定义** `asyncd_jobs`：`GET /asyncd/jobs` 返回
>    `{items:[{id,name,task_type,kind,schedule?,owner_syncer_id?,owner_name?,stats}]}`——
>    `kind='scheduled'` 带调度规则（`kind` ∈ `interval|daily|cron` + `interval_seconds`/`daily_times`/`cron_expr`/
>    `window_minutes`/`enabled`/`next_run_at`/`last_status`），`kind='async'` 没有 `schedule` 段。
>    定义有两类来源：
>    - **代码声明**（`owner_syncer_id` 为空）：启动时幂等 upsert，界面可启停；
>    - **数据声明**（`owner_syncer_id` 非空）：由**身份连接的「同步计划」**持有，一个连接一条，
>      `task_type` 固定为 `syncer.run`（到点即同步，一跳完成），`owner_name` 是连接名（界面按它显示来源）。
>    目录来自定义表，因此"注册了但从未执行过"的任务也在列（`stats.total=0`）；
>    代码里删掉的声明会**退役**（保留历史执行记录，不再出现在目录里）。
>    `PUT`（按数字主键）只用于启停**代码声明**的定时任务（`{"enabled":bool}`）；
>    对异步任务返回 `INVALID_PARAMS`（停用异步任务等于让业务动作静默丢任务），
>    对归属连接的任务同样返回 `INVALID_PARAMS`（启停跟随连接的计划）。
> 2. **执行实例** `asyncd_job_runs`：`GET /asyncd/jobs/:id/runs` 一行 = 一次执行，状态
>    `queued`（等待/退避）· `running`（执行中，持租约）· `succeeded` · `dead`（死信，可人工重试）；
>    `counts` 是与列表**同一作用域**的状态直方图（前端据此派生分页总数）。
> 3. **执行记录** `asyncd_job_run_attempts`：`GET /asyncd/jobs/:id/runs/:runId/attempts` 一行 = 一次尝试
>    （`running|succeeded|failed|lease_lost`，含真实耗时 `duration_ms` 与错误）。
>    重试会覆盖执行实例上的 `last_error`，所以"第几次失败、花了多久"只能看这里。
> 4. **执行器实例** `asyncd_workers`：`GET /asyncd/workers` 一个槽位一行（`id = <host>-<pid>#<slot>`），
>    `state` 由**心跳**算出：`online`（30s 内有心跳）· `stale`（心跳过期，疑似崩溃）· `stopped`（优雅退出），
>    `running` 是该槽位当前持租约在执行的任务数（实时聚合，不落列）。
>
> 任务定义由代码声明（平台任务）或由连接保存时写入（连接的计划）并自动注册；
> `last_status` 的 `skipped` 表示"单飞键命中：窗口内已有待执行/执行中的实例，本次跳过"——
> 不再把跳过误报成"已入队"。`cron` 为**标准 5 段**表达式（分 时 日 月 周，支持 `*`/列表/区间/步长；
> 不支持秒与 `JAN`/`@daily`/`L`）；非法表达式保存时被拒，历史脏数据会让该任务记 `failed` 并跳过调度。
> 身份连接的「同步周期/定时校验」与手工「立即同步」**共用单飞键** `syncer:<id>`：
> 两者重叠时只跑一次，定时侧如实记 `skipped`。
### 3.9 平台接入信息与系统设置

| 方法 | 路径 | 权限 |
|------|------|------|
| GET | `/system/oauth` | 平台管理员 |
| GET / PUT | `/system/storage` | 平台管理员 |
| POST | `/system/storage/test` | 平台管理员 |

> `/system/storage` 是**平台默认对象存储**（组织未配置 storage 提供商时的回落目标）：
> `type` ∈ `local` / `s3` / `minio`，`secret_key` 为 write-only（留空＝保持原值，响应只回 `has_secret`）。
> **`type` 为空串 = 显式"未配置"**：整行清零，此后未配置存储提供商的组织上传文件会得到 `PROVIDER_UNAVAILABLE`。
> `POST /system/storage/test` 写入并删除一个探测对象做连通性自检；未配置时同样返回 `PROVIDER_UNAVAILABLE`。

### 3.6 供给（连接器 / 策略 / 应用账号 / 历史）

| 方法 | 路径 | 权限 |
|------|------|------|
| GET | `/connectors/types` | `connector:read` |
| GET/POST | `/connectors` · GET/PUT/DELETE `/connectors/:name` | `connector:*` |

> **对齐检查**：`POST /connectors/:id/reconcile` 传 `{"application_id":1,"dry_run":true}` 只返回报告——
> 逐个核对本地应用账号在下游是否真实存在，结果分三类：`ok`（确认存在）/ `missing`（确认缺失）/ `unknown`（无法判定）。
> **`unknown` 绝不被当作缺失**（避免权限不足被误判成缺失而重复建号）；只有显式传 `api_dry_run=false` 才会为
> "确认缺失"的账号入队补建号任务。本地账号口令**不落库**：`app_accounts` 只以 `has_password` 表达"建号时设置过初始口令"。
>
> 连接器请求体：通用字段（`name` / `display_name` / `application_id` / `type` / `state`）+ **类型专属配置**——
> 非敏感值放 `config`、敏感值放 `secrets`（键由 `/connections/catalog` 声明并校验，未声明即拒绝；
> 敏感键混进 `config` 同样拒绝）。响应只回 `config` 与 `has_secrets`。支持的类型与各自字段、
> 以及"支持哪些动作（口令/更新/停用/删除）"都在 `/connections/catalog` 里声明。
> 应用账号创建/重试提交 `{"application_id":1,"user_id":2}`；删除使用同名查询参数。`application` / `user` 在响应中仅用于展示。
| POST | `/connectors/:name/test` | `connector:update` |
| POST | `/connectors/:id/reconcile` | `connector:update` |
| GET/POST | `/account-strategies` · GET/PUT/DELETE `/account-strategies/:name` | `account_strategy:*` |
| GET/POST | `/application-accounts` · DELETE `/application-accounts` · POST `/application-accounts/retry` | `application_account:*` |
| GET | `/provisioning` | `provision_history:read` |

> **列表过滤**：`/account-strategies`、`/application-accounts`、`/provisioning` 均支持 `application_id`
> 过滤；`/provisioning` 另支持 `connector`（连接器名称）。
> **控制台信息架构**：三个视图不再各占一级菜单，统一收进「应用详情 › 账号供给」；
> 出方向身份连接卡片的「查看账号同步记录」按 `connector` 过滤同一份 `provision_history`。

---

## 4. 错误码

| 分类 | 码 |
|------|----|
| 通用 | `OK` `UNKNOWN` `INVALID_PARAMS` `UNAUTHORIZED` `FORBIDDEN` `NOT_FOUND` `CONFLICT` `INTERNAL_ERROR` `SERVICE_UNAVAILABLE` `TOO_MANY_REQUESTS` |
| 租户 | `INSTITUTION_REQUIRED` |
| 认证 | `INVALID_CREDENTIALS` `SESSION_EXPIRED` `SESSION_NOT_FOUND` `INVALID_TOKEN` `TOKEN_EXPIRED` `USER_DISABLED` `PASSWORD_EXPIRED` `MFA_REQUIRED` `MFA_INVALID` `MFA_NOT_BOUND` |
| 验证码 | `CAPTCHA_REQUIRED` `CAPTCHA_INVALID` `VERIFY_CODE_INVALID` `VERIFY_CODE_TOO_MANY_REQUESTS` `PROVIDER_UNAVAILABLE` |
| 授权 | `PERMISSION_DENIED` `ROLE_NOT_FOUND` `PROTECTED_OBJECT` |
| OAuth/OIDC | `OAUTH_UNSUPPORTED_GRANT` `OAUTH_INVALID_CLIENT` `OAUTH_INVALID_GRANT` `OAUTH_INVALID_REDIRECT` `OAUTH_INVALID_SCOPE` `OAUTH_PROVIDER_ERROR` `SSO_STATE_INVALID` `IDENTITY_NOT_BOUND` |
| 存储/DB | `FILE_SIZE_EXCEEDED` `DB_ERROR` `DB_DUPLICATE` `DB_CONSTRAINT` `DB_NOT_FOUND` |

> 错误码在 `internal/errors/codes.go` 单点声明，`value` 全局唯一（有单测保证）。前端按 `code` 做 i18n。

---

## 5. 分页与过滤

列表接口通用参数：`page`（默认 1）、`size`（默认 20，上限 20）、`keyword`（模糊搜索），部分接口支持 `institution` 过滤。响应 `data` 形如：

```json
{ "items": [ ... ], "total": 123, "page": 1, "size": 20 }
```

---

## 6. 敏感字段约定

密码哈希、TOTP 密钥、恢复码、客户端密钥（除应用管理端展示）、证书私钥、令牌明文、同步器/连接器口令
**一律不下发**；以 `has_*` 布尔表达"是否已配置"（如 `has_client_secret`、`has_credentials`、`has_private_key`）。
