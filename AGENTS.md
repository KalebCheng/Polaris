# Polaris 项目开发规范

> 参照 SkyPivot 的工程规范制定，并针对"身份认证平台"的领域特点调整。修改本文件前请先确认架构决策。

## 1. 项目概述

Polaris 是统一身份认证与访问管理平台（IAM / IdP），采用模块化单体架构：

- **后端**：Go + Gin，PostgreSQL（业务数据）+ Redis（会话/临时态）
- **前端**：React 19 + TypeScript + Vite + Tailwind CSS v4 + shadcn/ui
- **能力面**：参照 Casdoor（组织/用户/用户组/角色/权限/应用/**提供商**/证书/令牌/会话/审计/身份连接 + OIDC Provider）

**范围边界**：不实现 Casdoor 的商业模块（产品/订单/支付/定价/套餐/优惠券/订阅/交易/余额）；
不实现其管理工具中的「表单 Forms」（列表页列配置，属前端 UI 配置）与「工单 Tickets」（客服工单，非 IAM 数据面）；
不实现 SAML / CAS、WebAuthn、联合凭证（FederatedCredentials）与自定义 HTML/CSS 注入（后者为纯展示且有存储型 XSS 面）。

**"配置即生效"原则**：新增任何配置字段，必须同时接入运行时读取点，否则不得入库——
理由是"配了但不生效"比"没有该配置"更危险（2026-10 教训：应用策略字段曾入库但登录/发令牌链路未读取）。

## 2. 架构设计

### 2.1 存储分层

| 存储 | 职责 | 数据类型 | 路径 |
|------|------|----------|------|
| PostgreSQL | 业务主存储 | 组织、用户、身份绑定、应用、提供商、角色、权限、用户组、证书、令牌、审计、身份连接 | `internal/repository/postgres/*` |
| Redis | 临时/高频数据 | 会话（session）、用户会话索引、OAuth state、限流计数 | `internal/repository/redis/*` |

### 2.2 后端目录

```
cmd/polaris/                 # 入口（-migrate / -migrate-down）
internal/
├── app/                    # 装配与依赖注入（唯一装配点）+ 种子数据
├── auth/                   # 认证授权核心
│   ├── oauth/              # 作为 SP 的外部连接器（dingtalk/wechat/feishu/oidc）
│   └── oidc/               # 作为 IdP 的证书管理与令牌签发
├── config/  errors/  logger/  tracing/
├── handler/                # HTTP 处理器 + 响应 DTO（dto.go / dto_resource.go）
├── repository/             # models.go（实体）+ iface.go（接口）+ postgres/ + redis/
├── server/                 # 路由与中间件
└── service/                # 业务逻辑（syncer/ 上游数据源连接器；provisioner/ 下游供给连接器；
                            #   provider/ 提供商目录与运行时（catalog/runtime/resolver）；
                            #   connection/ 身份连接的目录 schema；
                            #   asyncd/ 任务底盘（四维度：任务定义 / 执行实例 / 执行记录 / 执行器实例，
                            #     DB 版 asynq 语义，多实例安全））
pkg/                        # dbclient / redisclient / migrator
migrations/                 # 单一最新基线（001_init.up/down.sql）
configs/                    # 配置占位符
scripts/smoke-test.sh       # 端到端冒烟测试
frontend/web/               # 控制台前端
```

### 2.3 提供商（Provider）模块约定（2026-10-13 第四阶段）

「提供商」统一承载**外部身份提供方**（第三方登录 / 标准 OIDC）、**通知与基础设施服务**
（邮件 / 短信 / 人机校验 / 存储）与 **MFA**（TOTP / 短信）六类能力，**控制台是唯一维护入口**：

- 类别 / 类型 / 字段 schema 的唯一声明处是 `internal/service/provider/catalog.go`；
  请求体契约固定为 `config`（非敏感）+ `secrets`（敏感明文，留空=保持原值）+ `policy`（SSO 信任策略），
  逐键校验、未知键拒绝；**新增一个提供商类型只需改目录声明**，前端按目录渲染，不再硬编码。
- **存储按类别拆子表**：`providers` 只保留公共身份与状态；配置落在 `provider_oauth` / `provider_email` /
  `provider_sms` / `provider_captcha` / `provider_storage` / `provider_mfa`，仓库层聚合为 `repository.Provider`。
- **消息模板按渠道拆表**：`provider_email_templates`（subject/content）与
  `provider_sms_templates`（template_code/template_params）；应用登录方式与短信 MFA 只引用对应渠道的模板主键。
- **MFA 按登录方式配置**：`mfa_enabled` 控制是否进入 MFA，`mfa_required` 控制未绑定用户是否必须完成绑定；
  已绑定用户始终校验，未绑定且非强制时每次登录提示绑定但可跳过。组织级 `mfa_policy` 已移除。
- 运行时统一经 `provider.Runtime` 取配置（含密钥解密），经 `provider.Resolver` 构造外部 IdP 连接器；
  因此**改配置无需重启**。`configs/app.yaml` 的 `auth.oauth.providers` 仅作启动期种子。
- 密钥一律 AES-GCM 入库，响应只回 `has_secret`；存量明文用 `polaris -rotate-provider-secrets` 回填。
- **消息模板变量按声明装配**：短信模板的 `template_params` 声明模板内容用到的 `${变量}`（内置变量集见
  `internal/service/provider/template.go`：code / minutes / phone / institution / application），
  发送时**只装配声明过的**——阿里云对 `TemplateParam` 的变量名是严格匹配的（多一个少一个都会拒绝），
  未知变量名在落库前就被拒绝；邮件模板使用 `subject` / `content`，魔法链接额外支持 `link`。
- **单例类别**：运行时"取一个用"的类别（`captcha` / `storage`）在同一组织内
  **最多一个启用项**（目录 `CategoryInfo.Singleton` 声明 + 服务层校验 + 迁移 028 的部分唯一索引兜底），
  界面上以「当前生效」徽标 + 一键启用/停用表达；`email` / `sms` / `mfa` / `oauth` 都可并存多个，
  由应用登录方式或 MFA 提供商显式选择。
  动机：历史上多个同类启用项并存时，运行时按 `ORDER BY category, name` 取第一个，界面上完全看不出来。

## 3. 后端编码规范

### 3.1 红线（不可违反）

1. **领域组件禁止全局单例**：Service / Repository / Handler 必须构造函数注入，在 `internal/app/app.go` 统一组装。
   仅"跨切面基础设施"（`logger`、`tracing.Setup`、配置）允许启动时初始化一次且必须集中一处。
   判据：「需要 mock / 可能多实例 / 有业务依赖」→ 注入；「进程唯一 / 无业务状态」→ 启动引导一次。
2. **禁止跳过 Repository**：业务代码必须经 Repository 访问存储，不得直接使用 `dbclient`/`redisclient`。
3. **禁止 JWT 携带完整信息**：JWT **仅**携带 session id，完整认证信息（用户、角色、有效权限）存 Redis 会话。
4. **禁止跨层调用**：Handler → Service → Repository，不得反向或跳层。
5. **禁止提交真实密钥**：`configs/app.yaml` 只允许占位符，密钥必须经 `POLARIS_*` 注入（见 §3.5）。
6. **禁止非事务多表写入**：涉及多表的写必须使用 `dbclient.WithTx`（如"创建用户 + 绑定外部身份"、
   "更新密码 + 写入密码历史"、"角色关联整体替换"）。
7. **禁止向客户端下发敏感字段**：密码哈希、TOTP 密钥、恢复码、客户端密钥、证书私钥、令牌明文、
   同步器口令**一律不得出现在响应体**中。响应必须经 `internal/handler` 的 DTO（以 `has_*` 布尔表达"是否已配置"）。
8. **敏感端点必须限流**：登录入口启用失败计数限流（Redis）。
9. **修复必须带回归测试**：bug 修复必须附带测试；新增中间件/错误处理/配置逻辑/DTO 必须同步补测试。
10. **禁止未接线的死代码**：新增组件/页面/路由必须接入调用点（路由 + 导航）；公开 API 不得"已实现但从未被调用"。
11. **权限变更必须使会话失效**：角色/权限/用户组/用户状态变更后，受影响用户的 Redis 会话必须立即清除。

### 3.2 命名规范

- 包名：全小写、简短、无下划线（`auth`、`config`）
- 文件名：蛇形（`user_repo.go`、`auth_service.go`）
- 类型名：大写驼峰（`AuthService`、`SessionManager`）
- 接口名：大写驼峰（`UserRepository`）
- 方法名：大写驼峰、动词开头（`Create`、`GetByName`）
- 常量：全大写 + 下划线（`SessionKeyPrefix`）
- 错误变量：`Err` 前缀（`ErrNotFound`）
- 表名/列名：蛇形、复数表名（`user_identities`、`syncer_runs`）
- **表名禁止缩写**：一律用完整单词（`application_accounts` 而不是 `app_accounts`、
  `account_strategies` 而不是 `acct_strategies`）；例外仅限迁移 001–034 已发布的历史表，
  重命名随所属模块的下一次结构变更一并补（见 §3.8 的存量清单）。

### 3.3 错误处理

业务码类型为 `errors.Code`（`value`/`message`/`httpStatus`/`description`），
在 `internal/errors/codes.go` 用 `defineCode(...)` **单点声明**，`value` 全局唯一
（由 `codes_test.go` 的唯一性与完整性测试保证）。

- `message` 为下发客户端的**英文**文案（前端按 `code` 走 i18n）；`description` 仅开发用、不下发。
- 仅当调用方需要针对该 code 分支处理时才新开业务码，否则复用通用码。
- 包装底层错误用 `errors.Wrap(code, msg, err)`——底层 error **只进服务端日志**，绝不进响应体。
- 数据库错误统一经 `internal/repository/postgres.mapError` 转换（`23505`→`DB_DUPLICATE`、
  `23503/23514`→`DB_CONSTRAINT`、`pgx.ErrNoRows`→`DB_NOT_FOUND`、其余→`DB_ERROR`）。

### 3.4 HTTP 响应格式

```json
{ "code": "OK",             "message": "ok",                 "data": {} }
{ "code": "INVALID_PARAMS", "message": "invalid parameters", "details": {} }
```

- 成功：`errors.RenderSuccess(c, data)`（201 传 `http.StatusCreated`）。
- 失败：`errors.Fail(c, err)` + `return`，由 `errors.Middleware()` 统一渲染。
- 中间件拒绝：`errors.AbortError(c, err)`。
- handler **二选一**：写成功响应或 `Fail` 后返回，绝不同时写。
- **错误响应禁止携带堆栈/内部细节**：非 `*errors.Error` 一律兜底 `INTERNAL_ERROR`，不把 `err.Error()` 下发。
- 请求关联：`otelgin` 生成 trace，`X-Polaris-Request-Id` 回写响应头；响应体不携带 request_id。

### 3.5 配置与密钥

`configs/app.yaml` 只允许占位符；敏感/环境相关项通过 `POLARIS_*` 注入（`applyEnvOverrides`）。
**新增配置项一律同步支持环境变量覆盖**，不以"非敏感"为由排除。完整变量清单见 README。

默认配置语义（防再犯）：

- `security.csrf_allowed_origins` 与 `cors.allow_origins` **语义独立**：前者控制"谁能携带 Cookie 环境凭证执行写操作"，
  后者控制"谁能读取跨域响应"；**空 = 跳过校验（仅限开发），生产必须显式配置**；
  禁止用改 CORS 的方式悄悄关闭 CSRF。
- CSRF Origin 校验**只对依赖 Cookie 环境凭证的写请求生效**：显式携带 `Authorization` 的请求无环境凭证，跳过校验。
- 生产环境（`app.env=production`）强制要求 `POLARIS_CREDENTIAL_ENC_KEY`。

### 3.6 仓库行映射（避免列清单漂移）

实体列数在 40 列以上时，**必须**使用 `internal/repository/postgres/mapper.go` 的反射式映射
（依据 `db` tag 自动推导 SELECT/INSERT/UPDATE 列清单），禁止手写三份列清单——
历史教训：应用实体扩容时三次出现"列清单与参数不一致 / 漏列导致 23502"。

约定：`db:"-"` 表示非持久化字段；nil 切片与 nil map 由 `rowArgs` 统一归一化为空集合；
`updateEntity` 自动排除 `id` / `created_at` / `updated_at`（并统一写入 `updated_at = NOW()`）；
**`insertColumns` 同样排除 `created_at` / `updated_at`**——时间戳只能由数据库默认值生成
（Go 零值时间一旦写进去就是 `0001-01-01`），插入后用 `RETURNING created_at, updated_at` 回填实体。

### 3.7 结构体与 DTO

- **模型分层**：实体（`repository.*`）用于存储；请求/响应用 DTO，字段按用途收敛，禁止跨层混用。
- **敏感字段必须 DTO 化**：见 §3.1-7；`has_*` 布尔替代明文。
- **JSON tag 一律蛇形**；可选字段 `omitempty`；字段顺序：核心标识 → 业务字段 → 元信息/时间戳。
- **响应中 nil 集合必须归一化为空集合**（`[]` / `{}`），避免前端判空分支。
- 实体新增/调整字段必须同步：迁移 → 仓库 CRUD → DTO → 前端 `types.ts` → 测试。

### 3.8 关联一律用主键（禁止业务键关联）

**规则**：表与表之间的引用，列必须是对方的主键：列名 `<对方单数>_id`、类型 `BIGINT`、带外键约束
（`ON DELETE CASCADE` / `SET NULL` 按语义选择）。**禁止**用 `name` / `code` / `client_id` / 邮箱等业务键做关联列，
也禁止"存一个字符串、心里当它是指向外键"。

真实事故：`app_accounts.application` / `account_strategies.application` / `connectors.application` 存的是
`applications.code`（字符串、无外键）；重命名迁移（013：`applications.name → code`）之后该列的含义**静默地**
从"应用名"变成了"应用标识"，而引用完整性从未建立——账号策略可以配到**不存在的应用**、应用删除后留下孤儿行，
迁移 018 之后连 `(institution, code)` 唯一约束都没了（code 可重名）。

- 新增/改造的关联列一律按本规则落库；DTO 对外仍可暴露 `name` / `code`（人读友好），
  但**存储只认主键**，服务层负责 `code → id` 解析，解析不到直接 `INVALID_PARAMS` / `NOT_FOUND`（不做隐式兜底）。
- 前端必须用下拉选择真实资源（`Combobox` / `MultiSelect`），**禁止让用户自由输入应用标识/账号名**。
- 存量待改造项必须按模块登记并单独迁移；核心授权关系（应用准入、用户角色 / 职务、
  职务成员、部门成员、角色 / 权限关联）已统一使用主键与外键。禁止在新代码中继续引入
  `name` / `code` / 邮箱等业务键关联。

### 3.9 注释规范

- 代码注释统一使用英文。
- 注释只解释“为什么”、约束、边界条件或非显然的业务语义，禁止复述代码字面行为。
- 导出的类型、函数和接口只在用途不明显时补文档注释；删除无信息量的注释和注释掉的代码。
- 修改代码时同步修正所在代码块的注释；不得让注释与实现漂移。

## 4. 认证授权流程

```
1. 登录 → PostgreSQL 校验凭证（bcrypt）
2. （可选）MFA：先返回 mfa.required，前端带 mfa_code 二次提交；或使用恢复码
3. 创建 Redis 会话（含用户、角色、有效权限"资源-动作-效果"）
4. 签发 JWT（仅 session id）→ HttpOnly Cookie（生产 Secure）
5. 后续请求：中间件解析 JWT → 载入会话 → Enforce 判权
```

**权限判定语义（前后端必须一致）**：管理员直通 → `Deny` 优先于 `Allow` → `resource`/`action` 支持 `*` 通配。

**有效权限来源并集**：用户直接绑定、经角色的用户、经用户组的用户；角色支持嵌套（递归继承子角色）。

**SSO（作为 SP）**：`Provider` 级配置独立信任策略（`auto_create`/`match_type`/`match_local_field`/
`match_syncer_id`/`match_syncer_field`/`match_provider_field`/`allowed_domains`/`redirect_uri`）；
身份链接匹配必须固定到一条入方向身份连接，禁止跨连接按同名 `external_id` 模糊命中；
未绑定身份时按「本地字段 = 提供商字段」做唯一性匹配，唯一命中自动绑定，多命中拒绝。
state 暂存于 Redis（一次性消费），登录后回跳地址经 `?from=` 传递并在服务端暂存，回跳目标必须是站内相对路径。

**SSO 回调地址**：默认 `/sso/callback`（前端中间页，挂在前端命名空间，不与 OIDC 协议路径 `/oauth/*` 冲突），
可用 Provider 级 `redirect_uri` 覆盖（各 IdP 后台登记的重定向地址须与之逐字符一致）；
**禁止把 `from` 等自定义参数拼进 redirect_uri**（IdP 精确匹配会拒绝）。

**OIDC Provider（作为 IdP）**：`authorization_code` 必须校验 `client_id` / `redirect_uri` 注册值 /
授权码一次性消费 / PKCE（`S256` 与 `plain`）；`refresh_token` 默认轮换（旧令牌立即失效）。

## 5. 数据库迁移

当前仓库采用单一最新基线：`001_init.up.sql` 与 `001_init.down.sql`；不再保留历史升级脚本。
迁移由内置 `pkg/migrator` 驱动（advisory lock 防并发，记录写入 `schema_migrations`）。

```bash
make migrate-up            # 应用全部未执行迁移
make migrate-down          # 回滚最近 1 步
# 或用环境变量控制启动时自动迁移：POLARIS_DB_AUTO_MIGRATE=true|false
```

- 基线变更必须先在一次性 PostgreSQL 上完成 up/down/up 验证；已有部署升级需单独走数据迁移流程。
- 约束（唯一 / 外键 / 检查）必须在迁移中声明，不依赖应用层兜底。
- 多租户隔离：业务表以 `institution` 作为归属维度并对租户建立外键（`ON DELETE CASCADE`）。
- `NOT NULL` 的数组/JSONB 列在仓库层必须用 `orEmpty` / `orEmptyMap` 兜底（历史教训：nil 切片写入触发 `23502`）。
- **`DROP COLUMN` 会连带删除依赖该列的约束/索引**（真实事故：018 删 `institution` 列时，
  `users` / `applications` / `groups` / `departments` / `syncers` … 的"租户内唯一"约束一并消失且从未补回）；
  改表结构的迁移必须显式重建这些对象，并补一条"关键约束清单"回归测试（列出必须存在的唯一/外键，迁移后断言存在）。

## 6. 启动与测试

```bash
# 后端
make dev / make build / make run
make test            # go test ./...
make vet / make lint
./scripts/smoke-test.sh   # 端到端冒烟（需后端已启动）

# 前端
cd frontend/web
pnpm dev             # :5173，/api 与 /oauth 代理到 :8100
pnpm test            # typecheck + vitest + build
```

## 7. 前端架构

### 7.1 技术栈

React 19 · TypeScript（strict）· Vite 8 · Tailwind CSS v4（`@tailwindcss/vite`）· shadcn/ui（Radix）
· lucide-react · next-themes · React Router v7 · i18next · Vitest

### 7.2 目录结构（Feature-Sliced Design）

```
frontend/web/src/
├── main.tsx / App.tsx / Router.tsx
├── pages/                    # 页面级组件（dashboard / profile）
├── features/
│   ├── auth/                 # 会话上下文、登录页、OAuth 回调、路由守卫、认证舞台（星场 + 玻璃面板）
│   ├── iam/                  # API 客户端、hooks、通用表格/字段组件、各资源页面
│   └── i18n/  theme/         # 语言与主题切换
├── widgets/layout/           # AppLayout / AppSidebar / MobileNav / NavList / SidebarFooter
│   └── nav-config.ts         # 导航结构单一来源（侧边栏与移动端抽屉共用）
├── shared/
│   ├── api/                  # HTTP 客户端（统一响应解包）+ 类型定义
│   ├── components/           # 跨 feature 组件（BrandMark / ConfirmDialog / CopyButton / RowActionsMenu
│   │                         #   / FormWizard / FormSection / SwitchField / TagInput / RepeatableList）
│   ├── i18n/                 # i18next 初始化与语言资源
│   ├── lib/                  # cn() / 格式化 / 错误文案映射
│   └── ui/                   # 组件封装层（红线：页面只允许从这里导入 UI）
└── styles/globals.css        # Tailwind 入口 + 设计令牌（品牌渐变同样在此定义）
```

### 7.3 前端红线

1. **禁止绕过 `@/shared/ui`**：页面与 feature 只允许从此导入 UI 组件，禁止直接使用第三方组件或裸 HTML 控件组合。
2. **禁止跨 feature 引用**：feature 之间通过 `shared/` 通信。
3. **禁止裸色值与任意字号/间距**：使用 Tailwind utility 或 `globals.css` 的语义化 CSS 变量。
4. **禁止直接操作 DOM**：必须由 React state 驱动。
5. **禁止硬编码文案**：统一走 i18n（`shared/i18n/locales/*`），中英双语必须同步新增 key。
6. **禁止未接线的死代码**：新增页面必须挂载路由与导航，否则删除。
7. **页面必须懒加载**：`Router.tsx` 中一律 `lazy()` 分包；构建产物任一分包超 500KB（minify 后）必须处理。
8. **不得依赖敏感字段**：前端不应渲染/存储密码哈希、密钥、令牌明文（仅一次性展示签发结果）。

### 7.4 设计语言

**Control Plane（精密控制台）**：**宝石蓝主色**（`--primary`）+ 浅蓝灰画布（`--canvas`）+ **反相墨色胶囊**
（`--ink`，专门表达"我在哪"）+ 青绿 / 琥珀 / 红作状态点缀；业务页保持安静克制，
品牌氛围（轻微呼吸的标识）**仅限登录与 OAuth 回调页**（`auth-scope`：白场 / 近黑舞台 + 墨黑主按钮）。
全部设计令牌、三档圆角（12 / 18 / 24）与三档投影定义在 `src/styles/globals.css`（明暗双值，唯一载体），
用法与清单见 `docs/design-language.md`。

**信息架构（2026-10 调整）**：组织是资源归属的唯一维度，因此导航分两层：

```
PLATFORM            平台级（跨组织）      Dashboard、Organizations、Roles、Permissions、Security、
                                        设置（存储设置 / 任务中心 asyncd / 审计日志）
<当前组织>          组织内（工作区）      Overview / Users / Groups / Departments /
                                        Applications（含「账号供给」：策略 / 应用账号 / 同步记录）/
                                        Providers（提供商）/ **Connections（身份连接：入方向同步 + 出方向供给）**
ACCOUNT             账号级                Portal（/portal/**：应用中心 / 个人资料 / 安全 / 身份绑定 /
                                        应用账号 / 会话 / 活动）
```

账号级能力都在**用户门户**里；控制台顶栏的账号菜单只放「跳到门户个人资料」这类入口，不重复实现一遍。

- **账号生命周期不是一组一级菜单**：策略 / 应用账号 / 同步记录按应用收敛到
  「应用详情 › 账号供给」；出方向连接卡片按 `connector` 查看同一份 `provision_history` 的
  「账号同步记录」；用户详情「关联」保留个人维度的只读视图。

- **组织内资源的地址由 URL 承载**：`/institutions/:inst/...`（可分享、可回退），
  页面通过 `useWorkspaceOrganization()` 读取并把组织同步进上下文；侧边栏切换器切换组织时保留当前子路径。
- **禁止把组织内资源做成全局菜单**：用户/用户组/应用/提供商/身份连接/角色/权限/证书/令牌/会话/审计都属于组织，
  只能在组织命名空间下出现；旧地址（如 `/users`）统一重定向到"上次访问组织"下的同名页面。
- **应用编辑为分步向导**：`/institutions/:inst/applications/new|:id`，创建与编辑**复用同一五步向导**
  （基本信息 / 登录方式 / 注册与自助服务 / 客户端与端点 / 安全与令牌），登录页预览用**按需弹窗**（不放常驻侧栏），
  **吸顶头部**同时承载步骤条与「上一步 / 下一步 / 提交」，滚动到表单深处时也始终在手边。
  创建成功后停留在完成页展示 `client_id` / `client_secret` 与端点。分步向导与表单基元见
  `docs/design-language.md`（`FormWizard` / `FormSection` / `SwitchField` / `TagInput`）。
- **应用标识是 `code` 不是 `name`**：`applications.code` 进 URL 与外部集成且创建后不可改，
  `applications.name` 是人类可读名称；登录方式以 `signin_methods` 为唯一数据源（前后端同一套判定）。
- **资源引用一律下拉选择**：多选用 `MultiSelect`（登录提供方、scope、grant types、令牌字段、字段编排），
  可搜索单选用 `Combobox`（签名证书、默认用户组）；禁止用大段勾选框平铺资源列表。
- **「设置」是运维能力的归属地，不是一级资源**：`/settings/storage | /settings/tasks | /settings/audit`
  「安全管理」下的证书 / 令牌 / 会话同属这一层。两组的清单都在 `widgets/layout/nav-config.ts`
  （`settingsTabs` / `securityTabs`，`SubnavPageTab` 一条同时声明**标签**与**页头文案**），
  由**组级布局** `features/iam/pages/{security,settings}-layout.tsx` → `subnav-page-layout.tsx`
  渲染页头（标题 / 描述 / 操作）+ 二级导航（与顶栏同一块滑动墨色胶囊）——页面只提供内容。
  二级导航**两种形态**由 `variant` 决定：`menu`（系统设置）＝**左侧纵向菜单**（`PageShell.sidebar`
  槽位，分组抬头 + 行首图标 + 行满宽）；`tabs`（安全管理）＝页头下方一条横向胶囊。
  纵向菜单与工作区资源侧栏渲染的是**同一个** `NavMenu`，横向胶囊是同一个 `NavPills`
  （`orientation` 只改位移轴与圆角）——选中态语言始终只有一套。**为什么非要这一层**：二级标签是"点一下换路由"的换页导航，
  标签若由各页自己渲染就会整块重挂载，滑块每次都是新元素、只能瞬时落位；
  放进布局后它跨路由常驻，位移过渡才成立（`subnav-page-layout.test.tsx` 锁定"切换后滑块是同一个 DOM 节点"）。
  页头操作按钮（新建 / 刷新 / 编辑态保存）仍由**页面**渲染，用 `usePageHeaderActions` portal 进布局页头
  （按钮与页面弹窗、`disabled` 状态耦合，提升上去只会同步出一份过期状态）。
  任务中心（asyncd）与审计日志**不再各占一个一级导航位**——它们是平台运维视角的能力，不是资源；
  旧地址 `/asyncd`、`/audit` 一律重定向到新的二级地址；快速跳转（`nav-search`）平铺设置的二级页，
  父级「系统设置」不单独列出（两条同址结果会撞 React key）。
- **任务中心按 asyncd 的四个维度展示**（`/settings/tasks`，三个 Tab）：
  ①**任务维度**——异步任务（业务动作入队，无固定周期）与定时任务（代码声明的调度项，带频率/下次执行/启停），
  卡片给出四态执行统计（等待/执行中/成功/死信）与「最近入队」，任务目录来自后端 `GET /asyncd/jobs`
  （**定义表是唯一来源**，因此"注册了但从未执行过"的任务也在列）；
  ②**执行记录**——点进任务看该任务的执行实例列表（一次执行一行，按 `job_id` 过滤），
  死信行内可人工重试；③**尝试明细**——某次执行展开看每次尝试的状态/耗时/错误
  （重试会覆盖执行实例上的 `last_error`，只记在实例上就永远看不到"第几次失败"）；
  ④**执行器**——每个槽位一行（`<host>-<pid>#<slot>`），状态 `online|stale|stopped` 由**心跳算出**、
  `running` 实时聚合，于是"几个服务在跑、谁掉线了、各自在跑几个"都能直接回答。
  归属与状态都落到列上（`job_runs.job_id` / `attempts.job_run_id` / `workers.id`），**不靠 `unique_key` 前缀这类字符串约定**。
  定时任务的规则有 `interval` / `daily` / **`cron`**（5 段表达式，复用 `internal/service/cron`），并按**来源**过滤：
  平台声明的任务（界面可启停）与**身份连接的「同步计划」**（`owner_syncer_id` 非空，一个连接一条、
  `task_type=syncer.run` 到点即同步一跳完成、启停跟随连接、界面只显示来源不提供开关）。
  连接的计划与手工「立即同步」**共用单飞键** `syncer:<id>`，重叠时只跑一次；
  代码里删掉的任务声明走**退役**（`retired_at`，保留历史执行记录），不是删除。
- **单例配置页对齐组织级提供商**：系统设置 › 存储设置用**实例卡片**讲清"当前是哪一个、谁在用它、
  有哪几个行内动作"（测试连通性 / 修改配置 / 清除配置），类型与图标取自提供商目录
  （`GET /providers/catalog` 的 `storage` 类别，前端不再抄一份类型表）；点「修改配置」在**页内**展开分区表单
  （类型决定字段集合，故不切步骤、不开弹窗）。
  **清除 = 显式"未配置"**：`PUT /system/storage` 的 `type` 为空即整行清零（组织未配置存储提供商时
  文件上传才会失败），不允许出现"界面写着未配置、库里还留着旧值"。

**框架布局（2026-10-15 对齐实现）**：

- **顶栏＝悬浮玻璃岛**（`widgets/layout/top-bar.tsx`）：桌面端顶部承载**平台级导航**
  （Dashboard / Organizations / Roles / Permissions / Security / 设置）。
  它**不贴边**——左右留白与内容区**同一套刻度**（`px-4 sm:px-6 lg:px-8`）+ `pt-3` 露出画布，
  面板为 `rounded-2xl + bg-card/60 + backdrop-blur + shadow-soft`；
  改这套留白要同步 `platform-layout.tsx` / `workspace-layout.tsx` / `app-sidebar.tsx` / `portal-layout.tsx`
  （玻璃岛的边、内容卡片的边、侧边栏浮层的边必须落在同一条竖直线上）。
  选中的导航项是**反相墨色胶囊**（`bg-ink text-ink-foreground`）——由 `widgets/layout/nav-pills.tsx`
  渲染成**一块跟着选中项滑动**的滑块（`.sliding-pill-indicator`），切换走弹性缓动
  （令牌 `--motion-spring` / `--motion-pill-move` / `--motion-pill-size`）；
  选中项只改文字色，**不得再自己加 `bg-ink`**（会与滑块形成重影，`nav-pills.test.tsx` 锁定）。
  同一块滑块由 `shared/ui` 的 `useSlidingPill` + `SlidingPill` 提供，**分段控件 `Tabs` 共用**
  （提供商类别 / 身份连接方向 / 任务状态 / 用户详情分区；`tabs.test.tsx` 同口径锁定）；
  量测必须放在容器组件里，不能放进滑块子组件（子组件的 layout effect 早于容器 ref 挂载）。
  窄屏（`< md`）平台导航折叠为**同一玻璃岛内的第二行**（可横向滚动）。
- **工作区左侧栏**（`widgets/layout/app-sidebar.tsx`）：组织切换器 + 当前组织的资源导航，
  同样是画布上的白色圆角浮层（`NAV_PANEL_CLASS`：`rounded-2xl + shadow-soft`）。
  菜单条目与平台二级菜单是**同一份实现**（`widgets/layout/nav-menu.tsx` 的 `NavMenu`
  → `NavPills orientation="vertical"`），且**整块菜单只有一个容器**（分组抬头作为"不可选中的条目"
  一起进列表）——跨分组切换才是同一块滑块的位移；每个分组各渲染一份列表就只会"旧滑块卸载、
  新滑块就位"，看着像没动效。选中态同样是那块**会滑过去的墨色胶囊**：
  横向（顶栏 / 二级标签）与纵向（工作区侧栏 / 系统设置菜单）共用一处量测、一套令牌，
  切换时顺着位移就能看出"我从哪来"；**禁止**再给条目自己染底色（会与滑块形成重影）。
- **内容区贴边铺满**：`main` 只保留 `px-4 py-5 / sm:px-6 sm:py-6 / lg:px-8` 呼吸间距，
  不使用居中限宽容器（避免两侧大面积留白）。
- **全局入口在顶栏右上角的账号菜单**（`widgets/layout/account-menu.tsx`）：语言 / 主题 /
  个人资料（跳 `/portal/profile`）/ 用户门户 / 退出登录。组织归属由工作区侧边栏顶部的
  组织切换器承载，不在账号菜单里堆叠。
- **移动端**（`< md`）侧边栏不可见：工作区导航由玻璃岛左侧的菜单按钮打开抽屉，
  复用同一套 `WorkspaceNav` 与导航结构，因此**不存在两处维护**。
- **品牌标识**：`shared/components/brand-mark.tsx`（光球 + 北极星矢量，16px 可辨）
  与 `shared/components/brand-lockup.tsx`（字标：光球替换字母 `o`）是唯一载体，
  顶栏 / 门户 / 登录页页眉共用；`public/icon.svg` 与 favicon 用同一造型。
- **品牌作用域（品牌来源按页面层级决定）**：`features/iam/lib/brand-scope.ts` 是唯一判定入口——
  **管理端控制台一律使用平台内置品牌**（含平台层的 Dashboard / 组织 / 角色 / 权限 / 安全 / 审计 / 设置，
  **也含**点进组织后的工作区 `/institutions/:inst/**`；控制台是平台自身的产品界面）；
  只有**面向终端用户的入口**才体现租户品牌：用户门户 `/portal/**` 用所属组织 Logo/Favicon，
  登录页 / 注册页 / 魔法链接页由 `/auth/app-info` 按应用与组织直接渲染。
  禁止用"上次访问过的组织"（localStorage）驱动控制台品牌——那会让管理员在管理端看到某个租户的图片。

## 8. 工程教训

| 问题 | 根因 | 防再犯规则 |
|---|---|---|
| `/me` 等接口返回密码哈希 | 直接序列化实体，未经 DTO | §3.1-7：响应必须经 DTO，敏感字段以 `has_*` 表达 |
| Cookie 会话 401（Bearer 正常） | 把 Cookie 里的 JWT 当成 session id 直接查会话 | 统一先解析 JWT 再取 session id |
| 登出后仍可访问 | 登出把 JWT 当 session id，未真正删除会话 | 登出/状态查询统一走会话令牌解析 |
| 创建应用/权限/用户组报 `DB_ERROR` | nil 切片/映射写入 NOT NULL 数组或 JSONB 列 | §5：仓库层统一 `orEmpty` / `orEmptyMap` |
| 应用 `redirect_uris` 绑定失败 | 实体缺 json tag，前端蛇形字段无法绑定 | §3.6：实体与 DTO 一律显式蛇形 json tag |
| 登录/SSO 路由 404 | 只注册了受保护路由，遗漏公开端点 | 新增端点后立即补充冒烟用例（§3.1-10） |
| 非 IAM 能力膨胀 | 照搬 Casdoor 全部菜单（含表单/工单/商业） | §1 范围边界：能力面按需裁剪并记录理由 |
| 应用策略字段"配了不生效" | 模型与迁移先行，运行时未接线 | §1"配置即生效"原则：字段与读取点同批交付，并用冒烟用例覆盖 |
| 迁移回填已发布文件导致列缺失 | 已应用的 002 被追加 ALTER | §5 迁移不可变：遗漏一律新增迁移（如 003） |
| 手写列清单漂移（三次） | 实体膨胀后三段清单不同步 | §3.6：>40 列实体改用反射映射；`mapper_test.go` 锁定 id 排除/时序列/空集合归一化 |
| i18n key 覆盖页面标题 | 字段标签复用了页面级 key | §7.3-5：新增字段标签使用独立 key（如 `applications.appTitle`），`i18n.test.ts` 锁定中英一致 |
| 界面出现原始 i18n key | 代码引用了不存在的词条，测试只校验中英一致、未校验引用有效 | `src/shared/i18n/usage.test.ts` 扫描源码引用并断言词条存在 |
| 上传的头像/Logo/站点图标在开发环境全部裂图 | Vite 开发代理漏了后端自有的 `/static` 前缀：相对地址 `/static/storage/...` 在 5173 命中 SPA 回退返回 index.html（直连后端却是 200），生产 nginx 有该映射，故只在开发环境暴露 | 前端开发代理必须覆盖**全部**后端自有前缀（`/api`、`/oauth`、`/.well-known`、`/static`）；`vite.proxy.test.ts` 锁定该清单，新增后端自有前缀时同步补代理 |
| 字段标签变成「端点 / 根目录 / 网关地址」这种多义拼盘 | 同一个列名（`endpoint`）在不同类型下含义不同，却只写了一条通用词条；把三种语义硬拼成一个标签 | 词条解析顺序固定为**类型专属 → 通用 → 目录声明**（`providers.field.<type>.<key>`）；通用词条只写"多数类型下都成立"的含义，含义随类型变化时必须给类型专属词条（`SchemaFields` 已支持，`schema-fields.test.tsx` 锁定回退顺序） |
| 界面显示 i18next 报错原文（`key 'x' returned an object instead of string`） | 词条被多嵌套了一层（`providers.field.name` 写成 `{name, nameHint}`）；且测试用 `i18n.t(key)` 断言，坏掉时页面与断言拿到同一条报错字符串而"自证通过" | 字段类词条必须解析为**字符串**：后端 `internal/service/provider/i18n_test.go` 按目录声明锁定 `*.field.<key>` 的中英词条；前端断言一律写字面量，不用 `i18n.t` 与自身互证 |
| 组织内资源被做成全局菜单 | 照搬 Casdoor 的扁平菜单，未按归属分层 | §7.4 信息架构：组织内资源一律挂在 `/institutions/:inst` 下 |
| 系统设置只有一个裸表单，而任务中心 / 审计日志各占一个一级导航位 | 设置页按"字段表单"来做（没讲清"这是什么 / 谁在用 / 能做什么"），运维入口又与资源入口混在同一层导航 | §7.4：运维能力（任务中心 / 审计日志）收进「设置」二级页，旧地址重定向；单例配置页用实例卡片（状态 + 行内动作）＋页内分区编辑，类型清单走后端目录 |
| 界面上的「未配置」存不下去（选了也报必填错误），旧配置还留在库里 | 空类型被 `normalizeStorageType` 补成 `local`，随后又走 `local` 的必填校验 | 显式"未配置"必须有服务端语义（`type` 为空＝整行清零）＋回归测试；"看起来没配、其实还配着"比报错更危险 |
| 头像上传只有后端无入口 | 后端能力先行，前端未接线 | §3.1-10：新增能力必须同时接入调用点（弹窗内更换头像） |
| 双语 key 漂移 | 只补中文、遗漏英文导致回退显示 | `shared/i18n/i18n.test.ts` 锁定中英 key 集合一致 |
| 按用户过滤应用账号/供给历史静默返回空 | 动态过滤用未加引号的保留字列名（`user`）导致 SQL 解析失败 | 仓库层统一经 `columnExpr` 给保留字列名（`user`/`table`/`group`…）加引号 |
| 新增下游连接器的配置字段被静默丢弃 | 连接器请求只绑定实体上的固定字段，新增类型的字段（如目录服务的整套连接参数）传上来也不会落库 | 配置契约改为 schema 驱动：请求体为 `config`（非敏感）+ `secrets`（明文敏感值），键由类型目录声明并逐键校验，未知键/敏感键混入 `config` 一律拒绝；新增连接器类型不再需要改表或改实体 |
| 进程内队列满直接丢任务 | 供给用的是带缓冲 channel，`default` 分支只打日志 | 统一走 asyncd（任务落库 + 领取式 worker + 退避重试 + 死信），禁止再用进程内队列承载业务任务 |
| 下游"无法判定"被当成"不存在" | 对齐检查只看"查不到就补建号" | 存在性判定必须是三态（存在/缺失/无法判定）；`unknown` 绝不触发写入，避免权限不足导致重复建号 |
| 向导每一步都出现「品牌主题」/「接入方」 | 分区与上下文卡片写在步骤条件之外，每步重复渲染 | §7.4 步骤内容必须写在步骤条件内；`application-wizard.test.tsx` / `connection-edit-page.test.tsx` 锁定"只在所属步骤出现" |
| 登录页第三方登录是一排文字按钮「使用 X 登录 登录」 | 公开提供方列表只下发 `name`/`display_name`，前端**拿不到类型**就没法选 logo；文案还因 display_name 里已含"登录"而重复 | 公开面（`/auth/providers`）必须下发 `type` + `icon`（由提供商目录声明）；登录页用**品牌 logo 按钮**（`ProviderIcon`，组织命名作可访问名称/tooltip，缺 type/icon 的老数据退化占位图形）；素材缺失时补 `public/icons/connections/*` 并在 `SOURCES.md` 记录来源 |
| 需要第二因子时把「动态码」塞进凭据表单，用户得把账号密码再提交一遍 | 第一屏校验返回 `mfa.required` 后，只在同一个表单里多渲染一个字段（没有"切屏"这一步） | 第二因子必须是**独立一屏**（对齐 Casdoor `MfaVerify`）：凭据保存在 state、只补动态码；验证码输入为**6 格分段**（`shared/ui` 的 `InputOTP`：每格独立圆角、彼此有间隙；支持整串粘贴）并自动聚焦、**填满即自动提交**，另有 Enter 提交 + 「记住此设备」+「使用恢复码 / 返回上一步」；**第二因子屏不再显示第三方登录入口**（避免"可以绕开二次验证"的错觉）；切屏时刷新一次性的人机校验挑战（`login-page-requests.test.tsx` 锁定） |
| 注册二次验证只给一串密钥/链接，用户不知道要扫码 | 绑定页只把 secret 与 otpauth URL 当文本渲染（验证器 App 的标准路径是**扫码**） | 二次验证绑定一律**二维码为主 + 密钥/链接兜底**（`shared/components/qr-code.tsx`）；登录强制绑定与门户自助绑定**两处入口必须同批改**（`login-page-requests.test.tsx` 断言二维码 svg 存在） |
| TOTP 标签在验证器里显示成「Polaris (2/ceshi@Polaris)」 | otpauth 的 `AccountName` 用了 `组织主键/用户名@平台名`，既混进主键又重复平台名 | 标签固定为 `<用户名>@<组织标识>`、发行方为平台名（`auth/totp_test.go` 锁定；同名用户跨组织可区分） |
| 打开登录页时同一接口被重复请求（status ×4、providers/app-info ×3） | ① "加载一次"的 effect 依赖了它自己写回的 state（`loadAppInfo` → `setInstitution` → 依赖变化 → 再请求一轮）；② StrictMode 在开发环境双调用 effect，把它们再翻一倍 | 引导请求（status / institutions / providers / app-info）必须**按 key 守卫**（`useRef` 记录已加载键，或 `bootstrappedRef`），loader 通过 ref 读"最新组织"而不是把它放进依赖；`login-page-requests.test.tsx` 在 **StrictMode 下断言每个接口恰好 1 次**（该用例已用"撤掉守卫→失败"验证过） |
| 登录/发码/注册请求里的组织参数被**静默忽略**（多租户下登错组织） | 公开面请求 DTO 的 tag 写成驼峰 `institutionID`，而前端发的是蛇形 `institution`；Go 的字段匹配不是"大小写模糊匹配"，键名不同即丢弃 → 组织参数从未生效，后端一直走"自动解析 → 默认组织"，表现为"明明填对账号密码却报账号或密码错误" | §3.7 JSON tag 一律蛇形，且**跨语言契约字段名必须与前端类型逐字一致**；改字段名后前后端各加一条"按契约取值"的断言（冒烟：`institution_id` 登录 200、`app-info` 必须含 `institution_id`）。同一请求**只传一个权威来源**（有主键就只传 `institution_id`，`institution` 仅供"手上只有标识"的旧链接），并断言请求体里不含另一个 |
| 不带组织的 SSO 必然 404（跨组织解析成死代码） | `provider.Runtime.ListActive` 把 `institutionID == 0` 当成错误直接拒绝，而调用方正是用它表示"跨组织"（登录页未指定组织 / 只带提供商名发起授权） | `0` 的语义要在**声明处写清**（0 = 跨组织，而非非法）；跨组织查找必须有测试（`runtime_test.go::TestListActiveAcrossInstitutions`） |
| 点击「编辑」进入的是"选择类型"页（提供商 / 身份连接） | 编辑地址只带 id（与 direction），而类型来自 URL 参数；负责取记录的 effect 又被 `!typeInfo` 挡住 → **死锁**：记录永远不取、类型永远为空，于是落到"选类型"分支 | 编辑路径的类型必须**来自记录**：先按 id 取记录 → 用记录里的 type 定位类型 → 再回填表单；"选类型"分支只在 `isCreate && !typeInfo` 时渲染；`provider-edit-page.test.tsx` / `connection-edit-page.test.tsx` 的"编辑路径"用例锁定（已验证改回旧写法即失败） |
| 应用「名称」每次保存都被覆盖成「标识」（"Net Bird" → "net-bird"） | 应用是唯一"标识(code)与名称(name)分离"的资源，但 handler 的 Update 里仍写着 `app.Name = codeByID(c)`（当时 name 还是标识）；顺带使"请求体不带 code 的部分更新"因 Code 为空而 404 | **路径 id 解析出的标识必须写回标识字段**：凡"标识与名称分离"的资源（应用 `code`/`name`），Update 一律 `app.Code = 标识`；服务层用 code 定位、name 独立可改；`application_handler_test.go` + 冒烟「名称与标识各自独立」锁定 |
| `/asyncd/*` 全部 500（空指针 panic） | asyncd 服务在 handlers **之后**构造，handler 拿到的是 nil（装配顺序错误） | 依赖注入的**构造顺序即契约**：handler 持有的是服务实例，被依赖的服务必须先于 handlers 完成装配；启动期不允许出现"半装配"对象 |
| 应用账号/供给链路全部 `42703`（列不存在） | 迁移 026 删了 `related_password` 并新增 `has_password`，仓库的 SELECT/INSERT/UPDATE 列清单只改了一半（扫描目标已改、列名没改） | §5：**迁移改列 ↔ 仓库列清单/实体 db tag 必须同批修改**，且用一条只读冒烟用例（列表/详情）覆盖；删列前先采样留痕 |
| 冒烟门禁长期红，后面几十条用例形同虚设 | 第 9 步 `GET /asyncd/tasks` 500 后脚本 `set -e` 直接退出，19–24 步（提供商/供给/清理）从未真正执行过，掩盖了两处生产级缺陷 | 门禁失败即最高优先级修复；断言不要绑死格式化输出（如 `map[...]`），改为对**响应体**做形状断言 |
| 死配置又一轮：`provision_mode`/`default_role` 在 yaml 里也没被读过 | 从参考产品照抄字段，未确认读取点 | §1「配置即生效」同样适用于**配置文件**：引入字段前先 grep 读取点，读不到的就不引 |

| 反射映射创建的行 `created_at` 全是 `0001-01-01`（应用 / 提供商实体） | `insertColumns` 只排除自增主键，把 Go 的**零值时间**当成显式值写进了 `created_at`/`updated_at`，绕过数据库 `DEFAULT NOW()` | §3.6 的映射器必须"时间戳只由数据库生成"：INSERT 排除 `created_at`/`updated_at`，并用 `RETURNING` 把数据库生成的时间回填实体（否则"刚创建的对象"在响应里也是零值）；`mapper_test.go` 加 `TestInsertColumnsExcludesTimestamps` 锁定 |
| 管理员保存用户资料后，头像 / 姓名 / 部门被清空 | handler 把请求体直接绑进实体、服务层整行写回；前端只提交 8 个字段 → 未提交字段被零值覆盖 | 「部分更新」必须是**两端契约**：请求体用显式 DTO（指针/可选字段 = 本次提交），服务层只覆盖非 nil 字段，前端只提交发生变化的键；用户更新回归用例见 `user-detail-page.test.tsx`（断言请求体只含改动字段） + 冒烟「PUT 仅提交 title 后未提交字段保留」 |
| 解除登录限制"删不掉"用户的失败计数 | 锁定键是 `polaris:login_fail:<组织>:<应用>:<账号>:<IP>`，此前没有任何枚举/清理入口，只能等 TTL | 需要"按前缀清理"的能力时，先看清键结构：Redis `SCAN` 的 MATCH 是 **glob**（用户名含 `*`/`?` 会过度匹配）→ 必须拿到键后**按分隔符切段精确比对**再删；`login_lockout_test.go` 锁定"含通配符的用户名不误删他人计数" |
| 用户详情页每切一次标签就多请求一次（StrictMode 下翻倍） | 标签内的 `useEffect` 直接发请求，StrictMode 双调用 effect | 组件内的"首次加载"必须按 key 守卫（`useOnceKey`）：key 相同不再触发、key 变化（换用户/翻页）才重新加载；Vitest 在 **StrictMode 下断言每接口恰好 1 次**（`user-detail-page.test.tsx`） |
| 前端门禁 `pnpm test` 长期非 0 退出（测试全绿但报 unhandled error） | jsdom 未实现 `document.elementFromPoint`，`input-otp` 的光标测量定时器在测试结束后抛未捕获异常 | 环境缺失的浏览器 API 一律在 `vitest.setup.mjs` 补桩（`matchMedia`/`ResizeObserver`/`elementFromPoint`）；门禁必须看**退出码**而不是"用例全绿" |

| 应用没配第三方登录，登录页却出现钉钉按钮 | 读取侧把"提供方列表为空"当成"全都要"（写入侧 `validateApplication` 与下发侧 `availableSigninMethods` 都按"空 = 一个都没有"实现），同一个语义三处不一致 | **空集合的语义必须全链路唯一**：写入校验、下发策略、读取列表三处要么都按"空 = 无"、要么都按"空 = 继承全部"，并在同一处抽出唯一判定函数；解析失败（无效 `client_id` / 不存在的应用）一律 **fail-closed**，绝不退化成"列出全部" |

| 用户表单四次返工（一列铺满 → 右侧空 1300px → 右栏两张说明卡 → 才算对） | 根因不是间距，而是**选错了容器**：20 个彼此无依赖的字段被塞进"分步向导"（一步两三个字段，宽屏永远铺不满），随后又在空白处摆说明卡"填空" | 选容器先问"阶段之间有没有依赖"：无依赖 → **单页多列分区表单**（`FormSection` 全宽、内部 `xl:grid-cols-4`，一行铺满）；有依赖（登录方式决定后续）才用 `FormWizard`；**禁止用说明卡填空白**（换一步就是废物）；`user-edit-page.test.tsx` 锁定"多列栅格 + 无步骤条 + 分区标题各一次" |
| "顺手"给共享 `FormWizard` 加 `widthClassName` 并改头部结构，直接搞坏应用编辑页（步骤名被截断、按钮换行、丢掉"第 x / y 步"） | 只想着"让这一页更好看"，把**共享基元**当页面局部改 | 改 `shared/` 下的基元前先想清楚影响面（向导页有应用 / 提供商 / 身份连接 / 机构 / 用户五处）；页面的版式需求用**页面自己的容器**实现，共享组件保持兼容；要改就必须把受影响的页面全部截图核对一遍 |
| 新建用户表单是"一堵墙"：字段全铺一列、输入框拉到两千像素宽、标题还出现两遍 | 把 `FormSection` 当成"自带栅格的分区"用（它其实只是卡片容器），也没给向导步骤内容加宽度约束；同时向导头部与分区各渲染了一遍本步标题 | 基元的使用契约写进 `docs/design-language.md`：字段必须自己套 `grid sm:grid-cols-2`、向导内容必须套 `max-w-*`、同一步里标题只出现一次（分区可省略 `titleKey`）；`user-edit-page.test.tsx` 锁定"字段成组在栅格内 + 有宽度约束 + 分区无标题" |
| 刷新或直接打开工作区深链（`/institutions/:id/users/...`）被弹回组织列表 | `WorkspaceLayout` 用 `!loading && !institutions.some(...)` 判定"组织不存在"，而组织列表**还没加载完时** `institutions` 就是空数组 → 误判；`InstitutionProvider.reload()` 也没在拉取期间置 `loading=true`，消费者无法区分"没加载完"与"加载完但没有" | 空集合/未完成态必须可区分：`loading` 要在拉取期间为真，且"不存在"的判定要加上 `institutions.length > 0`；`workspace-layout.test.tsx` 锁定"列表未就绪时不得跳转" |
| 顶栏导航的滑块能滑，照搬进分段控件后"整块消失" | 把量测逻辑写进了容器的**子组件**：React 的 layout 阶段先递归子树、再给 host 节点挂 ref → 子组件的 `useLayoutEffect` 里 `containerRef.current` 恒为 `null`，量测静默早退（不报错、也不渲染） | 量测归**容器组件**（`useSlidingPill`）、渲染归子组件（`SlidingPill`）；回归测试必须锁"渲染出唯一一块滑块"，只锁"选中项不带 `bg-ink`"会漏掉这类静默失败（`nav-pills.test.tsx` / `tabs.test.tsx`） |

| 应用的"谁能使用"配了却不生效：从应用登录页进来照样能进 | 应用驱动登录页只用 `client_id` **渲染**（品牌/登录方式），而登录请求体里**根本没有应用上下文**（`LoginPayload` 只发 `institution_id`，后端 `LoginRequest` 连 `client_id` 字段都没有）→ 后端一律回退"组织默认应用" | 公开面认证请求（login / codes / login-code / magic-link / signup / SSO 发起）都必须随请求下发**应用上下文**（`client_id` 优先、`application` 兼容旧链接），并各自有"请求体确实带上了"的断言（`login-page-requests.test.tsx`） |
| 把"受限应用配成空名单"变成全组织只能改库恢复 | 准入判定挂在"**回退解析**出的应用"上（没带应用上下文时后端会回退到 `default_application`），于是默认应用一配错，管理员也进不来 | 判定作用域固定为"**请求是否携带应用上下文**"：`client_id`/`application` 都有才判定，都没有 = 登录平台、**不判定**（`resolveAppContext`，配 `TestResolveAppContextScope` 锁定）。副作用是不做"管理员直通"（准入对所有人一致） |
| 受限应用被保存一次主表单就悄悄变成"全员可访问" | `PUT /applications/:id` 是**整行 UPDATE**且请求体不带 `access_mode` → 零值 `""` 落库：要么违反 CHECK 报 `DB_ERROR`，要么在仓库层被兜底成 `all` 而**放宽准入**（静默、且是安全方向） | 写入口唯一化：这类子资源列（`access_mode`）必须由专用端点写，整行 `Update` 里**从 existing 继承**并加回归测试（`TestApplicationUpdateKeepsAccessMode`）；仓库层禁止 `""→默认值` 兜底 |
| 向导新建流程里"可授权主体"选择器永远空转 | 候选端点按**应用**取（需要应用主键），而新建时应用还不存在 | 面向"尚未创建的资源"的选择器数据源必须按**组织**取（`GET /applications/subject-options`），并给应用管理员单独授权（`application:read`）而不是要求 `user:read`/`group:read`/`department:read` |

| 授权名单做成三个下拉多选，用户说"一点都不好用" | 把"名单型"配置当成了"字段型"配置：`MultiSelect` 适合**字段级**多值引用（scope、grant types、token fields），而"已授权了谁"是**名单**——下拉把全局藏进控件，看不到全貌、不能逐条核对与移除，新增还要在下拉里翻半天 | 名单型授权一律用**左类型（含各自数量）/ 右名单表格**的主从布局（`application-access-editor.tsx`），行内移除 + 「添加授权」弹窗选候选；判断口径：**用户需要"看全并单条处置"的就是名单，不是字段** |

| 选了一批用户保存后回来看还是"全员可访问"（配置像没生效） | 保存用的 `useCallback(fn, deps)` **漏掉了它读取的 state**（`access`）→ React 复用首次创建的闭包，提交上去的永远是初始值（`all` + 空名单）。页面回显正常、保存也"成功"（主表单确实写进去了），所以只有数据层面的授权被旧值覆盖 | handler 只要读取 state，该 state 就必须进依赖数组；表单类页面补一条"回显 → 改动 → 保存 → 断言提交体"的集成测试（`application-edit-page.test.tsx`，撤掉依赖即红），别只测纯函数与子组件 |
| 连接的「同步计划」与任务定义跨两张表，写一半就留下"保存成功但计划不生效" | 本仓库约定"一个仓库方法 = 一个事务"（服务不得直接用 `dbclient`，§3.1-2），因此保存路径**不做跨域事务** | 保存时**即时**写入任务定义（用户立刻看到生效）＋ 每分钟 `syncer.schedule.reconcile` 对账自愈（补建 / 覆盖漂移 / **停用而不是删除**）；冒烟锁定"保存连接 → 目录出现该来源任务 → 删除连接 → 无孤儿" |
| 混合版本部署时：旧实例把新任务类型打成死信，并按 interval 语义推进 cron 任务的 `next_run_at`（真实观测） | asyncd 的任务类型与规则由**代码**声明，新旧实例共享同一张表；旧实例不认识新类型（记 `没有处理器` 进死信）、也不认识 `cron`（回落 interval = 每分钟） | 发布顺序固定为"**停旧实例 → 迁移 → 起新实例**"，不要新旧并行；新增任务类型或调度规则时必须把这条写进发布说明 |
| 迁移里 `DROP INDEX asyncd_tasks_type_status_idx` 报 `42704 index does not exist`（第一次真实库执行 033 时） | 同一迁移前一步已 `DROP COLUMN type`，而 **PostgreSQL 会连带删除依赖该列的索引**（`job_name` 同理会带走 `job_idx`） | 迁移里所有 `DROP INDEX` 一律写 `IF EXISTS`（删列导致的连带删除不可预期）；改表结构的迁移**必须在真实库上跑一遍 up/down**，不能只做静态审查 |
| 任务中心只有"执行实例"，"任务定义 / 执行记录 / 执行器"三个维度全缺 | 一张队列表既当队列又当历史：任务清单取自"有执行记录的类型"（从未执行过的任务凭空消失）、重试覆盖 `last_error` 与 `finished_at`（第几次尝试失败、跑了多久查不到）、`locked_by` 只是个字符串（有几个实例在跑、谁掉线了答不了） | §7.4 + `docs/specs/261015-asyncd-task-model.md`：任务底盘固定四维度（定义 `asyncd_jobs` / 执行实例 `asyncd_job_runs` / 执行记录 `asyncd_job_run_attempts` / 执行器 `asyncd_workers`）；定义表是任务目录**唯一来源**（代码声明 + 启动 upsert）；每次尝试一行、执行器在线状态**由心跳算出**（不存派生列）；归属与状态一律落到列/外键上，不靠字符串约定 |
| 删掉最后一个子部门后，父部门**永远删不掉**（409 still has child departments） | `DepartmentService.Delete` 用**存储的** `has_child` 标记判断"还有下级"，而删除路径从未刷新过它——只有"移动子部门层级"（走 `Update`）才会顺带刷新，于是必须"先动一下再删"这么反直觉 | 派生/冗余标记必须在**每条会改变它的写入路径**上同步（本次在删除后补 `refreshHasChild`，配回归测试）；新增这类标记时先问"还有什么操作会改变它" |

| 钉钉一同步就多出 324 个用户，同一个人 6 行重复 | 迁移 018 用 `ALTER TABLE ... DROP COLUMN institution` 删列，PostgreSQL **连带删除**了该列参与的全部租户内唯一约束/索引（`users_inst_name_key`、邮箱/手机唯一索引、`applications` / `groups` / `departments` / `syncers` … 的 `*_inst_name_key`、`app_accounts_inst_app_user_key`），此后再无迁移补回；同时同步引擎按部门逐条拉取、整批计划内不去重 | 唯一性只能由数据库保证，且必须有测试盯着：补"关键约束清单"回归测试（迁移后断言约束存在）；删列/改名列的迁移一律显式重建依赖对象；入方向同步必须按"连接 + 源端 ID"去重（第六阶段 Spec） |
| 账号策略能配到不存在的应用，应用删了留下孤儿行 | `app_accounts.application` / `account_strategies.application` / `connectors.application` 存的是 `applications.code` 字符串且无外键（006 建表时语义是"应用名"，013 改名后语义静默变成"标识"），`CreateStrategy` 也不校验应用是否存在，前端是自由文本输入 | §3.8：关联一律用主键 + 外键；业务键只出现在 DTO 层；前端必须下拉选择真实资源 |
| 应用账号「删除」只删本地台账，下游账号仍在 | 服务层直接 `accounts.Delete`，绕过了 `processRecycle(delete)`，界面成功但下游没变 | 所有对外删除动作必须走**同一条下游回收链路**；回归测试断言"派发 delete 任务且成功前不删本地台账" |
| JIT 建号有两个开关（策略 `createWhen=jit` + 连接器 `just_in_time`），配一个不生效 | 运行时读连接器字段，界面却把策略当权威来源 | 同一语义只保留一个配置源；迁移时先把旧字段回填到新来源（无策略则补 JIT 策略、manual 则改 jit），再删旧列，并补"策略触发"单测 |

---
*最后更新：2026-10-16（新增 §3.8「关联一律用主键」与「表名禁止缩写」；§5 补「删列连带删除约束」；此前 2026-10-15 为 asyncd 四维度重建：`asyncd_jobs` / `asyncd_job_runs` / `asyncd_job_run_attempts` / `asyncd_workers`）*
