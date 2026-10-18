# Polaris

Polaris 是一个统一身份认证与访问管理平台（IAM / IdP）：以 **组织（Institution · 租户）** 为多租户隔离边界与资源归属维度，
集中管理用户、用户组、角色、权限、应用（OAuth2 / OIDC 客户端）、提供商与身份连接，
既能作为 **OIDC Provider** 对外提供统一登录，也能作为 **SP** 接入第三方身份源。

功能与能力面参照 [Casdoor](https://github.com/casdoor/casdoor) 设计；工程规范参照
内部 DevOps 平台 SkyPivot 的模块化单体架构。

## 能力总览

| 域 | 能力 |
|----|------|
| 身份目录 | 组织（多租户）、用户、用户组、外部身份绑定 |
| 访问控制 | 角色（支持嵌套继承）、权限（资源-动作，Allow/Deny，支持 `*` 通配）、鉴权测试 |
| 应用接入 | 应用（OAuth2 / OIDC 客户端）+ 完整运行策略：登录方式开关、会话数量、IP 白名单、登录失败次数限制与重试等待时间、字段编排、令牌有效期/轮换/字段白名单、应用级品牌主题、跳转地址 |
| 提供商 | **五类提供商**（**统一在控制台「提供商」中管理，改完即生效、无需重启**）：外部身份提供方（第三方登录：钉钉/微信/飞书/Lark，标准 OIDC：discovery 或自定义端点 + PKCE）、邮件 SMTP、短信（阿里云 / 通用 HTTP）、人机校验（本地图形 / hCaptcha / reCAPTCHA / Turnstile）、存储（本地目录 / S3·MinIO） |
| 作为 IdP | OIDC discovery / JWKS / authorize / token / userinfo / introspect / logout |
| 令牌签发 | authorization_code（含 PKCE）、refresh_token（轮换）、password、client_credentials |
| 安全能力 | bcrypt 密码策略与历史、会话（Redis）、TOTP 二次验证 + 恢复码、登录限流（平台默认 + 应用级覆盖）、CSRF、人机校验（按组织启用）、邮箱/短信验证码登录、应用级 IP 白/黑名单 |
| 组织维度 | 组织详情聚合视图（各资源规模 + 一键进入管理）、控制台按组织聚合、全局组织切换器 |
| 文件托管 | 用户头像上传（由存储类提供商承载），本地静态直出或对象存储直链 |
| 密钥与证书 | 证书管理（RSA 自签名，用于 id_token 签名）、JWKS 发布 |
| 审计 | 操作记录（含 trace_id）、令牌清理、记录清理 |
| 身份连接 | 外部数据源（钉钉 / 飞书 / 企业微信 / LDAP / 数据库，入方向）与下游连接器（出方向）|

### 明确不做

- **Casdoor 的商业模块**（产品 / 订单 / 支付 / 定价 / 套餐 / 优惠券 / 订阅 / 交易 / 余额）——不实现。
- **Casdoor 管理工具中的「表单 Forms」**：它是「列表页列显示配置模板」（`FormItems[{name,label,visible,width}]`），
  后端除自身增删改查外无任何流程依赖，本质是把前端 UI 配置塞进后端。列配置由前端代码直接表达即可，不引入。
- **Casdoor 管理工具中的「工单 Tickets」**：面向「用户提交诉求 + 管理员回复」的轻量客服工单
  （状态 `Open / In Progress / Resolved / Closed`，含 `messages` 会话），与 IAM 数据面无关，不做。
- **Webhooks / Webhook Events**：当前未实现；若要做「身份事件推送到外部系统」，建议作为独立模块补上。

## 技术栈

**后端**：Go 1.25+ · Gin · PostgreSQL 15+（pgx v5）· Redis 7+（go-redis v9）· zap · OpenTelemetry · JWT · go-oidc

**前端**：React 19 · TypeScript（strict）· Vite 8 · Tailwind CSS v4 · shadcn/ui（Radix UI）· React Router v7 · i18next · Vitest

## 快速开始

### 1. 启动依赖

```bash
# 方式一：Docker Compose（若端口未被占用）
make docker-up

# 方式二：使用已有的 PostgreSQL / Redis，按需创建库
#   PostgreSQL: 创建用户 polaris / 密码 polaris123 / 数据库 polaris
```

### 2. 启动后端

```bash
export POLARIS_JWT_SECRET=dev-secret-for-local-testing-only
make dev            # go run ./cmd/polaris，监听 :8100
```

首次启动会**自动执行迁移**并写入种子数据（内置组织 `built-in`、管理员 `admin`、内置应用 `app-built-in`、
内置证书 `cert-built-in`）。默认管理员口令为 `admin123`，可用 `POLARIS_ADMIN_PASSWORD` 覆盖；
生产环境请关闭种子或显式设置强口令。

```bash
make test           # 单元测试
make migrate-up     # 显式执行迁移
./scripts/smoke-test.sh   # 端到端冒烟测试
```

### 3. 启动前端

```bash
cd frontend/web
pnpm install
pnpm dev            # http://localhost:5173，/api、/oauth、/.well-known、/static 代理到 :8100
```

登录：组织 `built-in`、账号 `admin`、密码 `admin123`。

### 4. 用镜像部署（Docker Hub）

镜像把**后端 API + 前端 SPA + nginx**打包为单容器（nginx 托管静态资源并把
`/api`、`/oauth`、`/.well-known`、`/static` 反代到内部 Go 服务），因此单容器即可运行完整平台。

```bash
# 1) 启动依赖（PostgreSQL + Redis）+ 应用
export POLARIS_JWT_SECRET=dev-secret-for-local-testing-only
export POLARIS_CREDENTIAL_ENC_KEY=$(openssl rand -base64 32)   # 生产必须显式注入
docker compose up -d

# 访问 http://localhost:8100 （组织 built-in / admin / admin123）
```

也可直接用镜像运行（需自备 PostgreSQL / Redis）：

```bash
docker run -d --name polaris -p 8100:8100 \
  -e POLARIS_ENV=production \
  -e POLARIS_DB_HOST=your-pg-host \
  -e POLARIS_DB_PASSWORD=your-pg-password \
  -e POLARIS_REDIS_HOST=your-redis-host \
  -e POLARIS_JWT_SECRET=your-jwt-secret \
  -e POLARIS_CREDENTIAL_ENC_KEY=$(openssl rand -base64 32) \
  polaris/polaris:latest
```

镜像由 GitHub Actions 在打 tag 时自动构建并推送（`.github/workflows/release.yml`）：

```bash
git tag v0.1.0 && git push origin v0.1.0   # 触发多架构（amd64/arm64）构建 → Docker Hub
```

发布前需在仓库配置 Secrets：`DOCKERHUB_USERNAME`、`DOCKERHUB_TOKEN`；
镜像名可用仓库变量 `DOCKERHUB_IMAGE` 覆盖（默认 `<用户名>/polaris`）。

## 对外接口

### OIDC Provider 端点

| 端点 | 说明 |
|------|------|
| `GET /.well-known/openid-configuration` | 发现文档 |
| `GET /.well-known/jwks` | JSON Web Key Set |
| `GET /oauth/authorize` | 授权端点（要求本地登录态） |
| `POST /oauth/token` | 令牌端点（4 种 grant） |
| `GET|POST /oauth/userinfo` | 用户信息端点 |
| `POST /oauth/introspect` | 令牌内省 |
| `GET|POST /oauth/logout` | 结束会话 |

支持 **OIDC Back-Channel Logout**（应用配置 `backchannel_logout_uri` 时，登出即向下游发送 `logout_token`）
与 **应用适配器**（`adapter=default|custom` 驱动 id_token / userinfo 的 claim 映射）、**JWKS 多密钥平滑轮换**。

### 接入文档

| 文档 | 内容 |
|------|------|
| [OIDC 接入指南](docs/oidc-integration.md) | 把业务系统接成 OIDC 客户端（授权码 + PKCE、claims、登出、JWKS） |
| [API 参考](docs/api-reference.md) | 管理 API 清单与错误码 |
| [数据库表结构与 ER 图](docs/database-schema.md) | 业务数据表、字段、约束、索引与分域 ER 图 |
| [部署手册](docs/deployment.md) | 镜像部署、环境变量、迁移、密钥轮换 |
| [账号供给接入指南](docs/provisioning-connector.md) | 下游连接器（Connector）契约 |

### 控制台信息架构

```
PLATFORM   Dashboard（跨组织总览）  Organizations（组织列表 → 组织概览）
<组织>      Overview / Users / Groups / Applications / Providers（提供商）/
           External sources / Roles / Permissions / Certificates / Tokens / Sessions / Audit logs
ACCOUNT    左下角用户区（头像/姓名/邮箱 → 语言、主题、个人资料弹窗、退出登录）
```

组织内资源的地址形如 `/institutions/:inst/users`（组织由 URL 承载，可分享/可回退）；
切换组织保留当前子页面；应用编辑为单页分区表单（`/institutions/:inst/applications/new|:name`）。

### 管理 API（`/api/v1`）

统一响应体：成功 `{ code: "OK", message, data }`；失败 `{ code, message, details }`，
`code` 为字符串业务码（见 `internal/errors/codes.go`）。

```
POST   /auth/login                      # 密码登录（可带 mfa_code、captcha_id/captcha_code/captcha_token）
POST   /auth/login/code                 # 邮箱 / 短信验证码登录
POST   /auth/codes                      # 发送登录验证码（受频率限制，启用验证码时需带人机校验）
GET    /auth/captcha                    # 人机校验挑战（组织未启用时为 required=false）
GET    /auth/app-info                   # 登录页公开的应用策略（登录方式/字段编排/可用提供方）
POST   /auth/signup                     # 自助注册
GET    /auth/status                     # 会话状态
GET    /auth/providers                  # 可用登录提供方（读提供商表）
GET    /auth/oauth/:provider/authorize  # 发起 SSO
GET    /auth/oauth/:provider/callback   # SSO 回调
POST   /auth/mfa/{setup,verify,disable} # TOTP 二次验证
POST   /auth/change-password
GET    /me

GET|POST         /institutions         PUT|DELETE /institutions/:id
GET|POST         /users                 GET|PUT|DELETE /users/:id
                 # 用户详情（第五阶段）：资源标识统一用数字主键 id
                 /users/:id/security           # 密码 / MFA 状态 + 登录失败锁定条目
                 /users/:id/authorizations     # 直接角色 / 继承角色 / 用户组 / 有效权限（含来源）
                 /users/:id/roles              # PUT 整体替换直接角色
                 /users/:id/groups             # PUT 整体替换用户组归属
                 /users/:id/department         # PUT 变更部门归属
                 /users/:id/admin              # PUT 设为 / 取消管理员（含"最后一个管理员"防呆）
                 /users/:id/disable · /enable  # 禁用 / 启用（禁用即失效全部会话）
                 /users/:id/reset-password · /reset-mfa · /unlock
                 /users/:id/identities         # 外部身份绑定（DELETE 解绑）
                 /users/:id/app-accounts · /provision-history · /records   # 关联与活动
                 /users/:id/api-keys           # 签发 API 令牌
GET|POST         /departments           PUT|DELETE /departments/:id (/members, /reorder)
GET|POST         /applications          GET|PUT|DELETE /applications/:name (/reset-secret)
GET|POST         /providers             GET|PUT|DELETE /providers/:id
GET|POST         /roles                 GET|PUT|DELETE /roles/:name (/relations)
GET|POST         /permissions           GET|PUT|DELETE /permissions/:name (/subjects, /enforce)
GET|POST         /groups                GET|PUT|DELETE /groups/:id (/members, /detail)
GET|POST         /certs                 GET|PUT|DELETE /certs/:name
GET              /tokens                /tokens/:name (/revoke)
GET              /sessions              /sessions/:id · /sessions/revoke-all
GET              /records               /records/:id
GET|POST         /syncers               GET|PUT|DELETE /syncers/:name (/run, /runs, /types)

POST   /me/avatar                       # 上传当前用户头像（multipart，字段名 file）
GET    /providers/catalog               # 提供商目录（类别 / 类型 / 字段 schema；前端唯一来源）
POST   /providers/:id/test              # 提供商连通性自检（按类别真实探测）
GET    /institutions/:name/stats       # 组织资源聚合
GET    /institutions/stats             # 全部组织资源聚合
GET    /static/storage/*                # 本地存储文件只读直出（对象存储部署由对象存储域名提供）

**用户相关权限动作**：`user:read` / `user:create` / `user:update` / `user:delete`，
外加两个**必须独立授权**的安全动作——`user:reset_mfa`（重置二次验证，唯一能"降级第二因子"的动作）
与 `user:unlock`（解除登录失败锁定，典型 Helpdesk 能力：能解锁但不该能改用户资料）。
角色分配走 `role:update`，会话强制下线走 `session:delete`。内置 `admin` 与组织内最后一个管理员
不可被禁用 / 删除 / 取消管理员（服务层防呆，返回 `PROTECTED_OBJECT` 或 `CONFLICT`）。

GET    /healthz                         # 存活探针
GET    /readyz                          # 就绪探针（含依赖状态）
```

## 运行机制要点

### 认证流程

```
1. 登录请求 → PostgreSQL 校验凭证（bcrypt）
2. 需要 MFA 时先返回 mfa.required，前端二次提交 mfa_code
3. 创建 Redis 会话（承载用户/角色/有效权限）
4. 签发 JWT（仅含 session id）→ 写入 HttpOnly Cookie
5. 后续请求：中间件解析 JWT → 从 Redis 载入完整会话 → Enforce 判权
```

- **JWT 只携带 session id**，完整认证信息在 Redis 会话中（可即时失效）。
- 权限判定：`管理员直通` → `Deny 优先于 Allow` → `*` 通配；前后端语义一致。
- 权限/角色/用户组/用户状态变更后，相关用户会话**立即失效**。

### 提供商（外部身份提供方 + 基础设施服务）

**唯一维护入口是控制台「提供商」**（组织内 `Providers` 页面）：新建/编辑走整页向导，
字段由后端目录（`internal/service/provider/catalog.go`）声明——界面里出现的每个字段都在运行时真实生效。

- **配置契约**：请求体为 `config`（非敏感）+ `secrets`（敏感明文，留空 = 保持原值）+ `policy`（SSO 信任策略）；
  未知键与"敏感键混入 config"一律拒绝，不存在"传上来被静默丢弃"。
- **密钥**：AES-GCM 加密入库（`security.credential_enc_key`），响应只回 `has_secret`。
- **短信模板变量**：模板内容里用到的 `${变量}` 由提供商声明（`template_params`，可选 code / minutes / phone /
  institution / application）；发送时只按声明装配——阿里云对变量名严格匹配，多一个少一个都会拒绝。
- **单例类别**：`email` / `sms` / `captcha` / `storage` 在**同一组织内最多一个启用项**（否则"到底用哪个"取决于排序）；
  切换用列表卡片上的「启用 / 停用」，数据库层有部分唯一索引兜底；`oauth` 类别可并存多个启用项（登录页列出全部）。
- **外部身份提供方**：`category=oauth`（钉钉 / 微信 / 飞书 / Lark / 标准 OIDC）。改配置**无需重启**即生效；
  OIDC 走 discovery（或自定义授权/令牌端点）+ 可选 PKCE（S256）。
- **SSO 信任策略**：`auto_create`、`link_by_email`、`allowed_domains`、`redirect_uri`（回调覆盖）；
  落地组织即该提供商所属组织（不再依赖 `auth.oauth.default_institution`）。
- **迁移期兜底**：`configs/app.yaml` 的 `auth.oauth.providers` 仅作为种子——启动时若目标组织尚无同名提供商，
  会写入一次并打印日志；此后请改控制台。

### 外部数据源同步

连接器把外部系统归一化为「用户 / 用户组」视图，由同步服务统一写回：更新既有用户、创建新用户、
同步用户组成员；支持 `schedule`（秒）定时同步与手动触发，并记录每次执行结果（`syncer_runs`）。
敏感字段（口令、应用密钥）以 AES-256-GCM 加密落库，接口只返回"是否已配置"。

## 配置与密钥

`configs/app.yaml` **只允许占位符**，真实密钥与环境相关配置通过 `POLARIS_*` 环境变量注入。

| 变量 | 作用 |
|------|------|
| `POLARIS_CONFIG` | 配置文件路径（默认 `configs/app.yaml`） |
| `POLARIS_ENV` / `POLARIS_PORT` | 运行环境 / HTTP 端口 |
| `POLARIS_DB_HOST` `_PORT` `_USER` `_PASSWORD` `_NAME` | PostgreSQL 连接（密码敏感） |
| `POLARIS_REDIS_HOST` `_PORT` `_DB` | Redis 连接地址 / 库号 |
| `POLARIS_REDIS_PASSWORD` | Redis 密码（敏感） |
| `POLARIS_JWT_SECRET` | JWT 签名密钥（敏感，必须注入） |
| `POLARIS_CREDENTIAL_ENC_KEY` | 敏感字段加密密钥（base64 32 字节，生产必须注入） |
| `POLARIS_COOKIE_SECURE` | 会话 Cookie Secure（生产 true） |
| `POLARIS_CSRF_ALLOWED_ORIGINS` | CSRF 可信来源（逗号分隔；空 = 跳过校验，仅限开发） |
| `POLARIS_LOGIN_MAX_ATTEMPTS` / `_WINDOW_SECONDS` | 登录失败次数上限 / 冻结窗口（平台默认；应用可分别用 `signin_error_limit` / `signin_retry_wait_minutes` 覆盖，0 = 沿用平台默认） |
| `POLARIS_DB_AUTO_MIGRATE` / `_SEED` / `_MIGRATIONS_DIR` | 迁移与种子控制 |
| `POLARIS_ISSUER` | 作为 IdP 的 issuer（默认回退 `auth.oauth.base_url`） |
| `POLARIS_DEFAULT_INSTITUTION` | SSO 自动建号/关联的落地租户（默认 `built-in`） |
| `POLARIS_OIDC_ADDITIONAL_KEY_CERT_NAMES` | 附加校验密钥证书名（JWKS 多密钥轮换，逗号分隔） |
| `POLARIS_OAUTH_<PROVIDER>_CLIENT_SECRET` | 各外部 Provider 的 client_secret（敏感） |
| `POLARIS_ADMIN_PASSWORD` | 种子管理员口令（仅开发） |

前端环境变量见 `frontend/web/.env.example`（`VITE_API_BASE_URL`、`VITE_PROXY_TARGET`）。

## 目录结构

```
Polaris/
├── cmd/polaris/              # 应用入口（含 -migrate / -migrate-down）
├── internal/
│   ├── app/                 # 唯一装配点（依赖注入）+ 种子数据
│   ├── auth/                # 认证授权核心（会话/JWT/密码/TOTP/策略/中间件）
│   │   ├── oauth/           # 作为 SP 的外部连接器（钉钉/微信/飞书/OIDC）
│   │   └── oidc/            # 作为 IdP 的能力（证书管理 + 令牌签发）
│   ├── config/              # 配置加载与 POLARIS_* 覆盖
│   ├── errors/              # 业务码、错误包装、统一响应
│   ├── handler/             # HTTP 处理器 + 响应 DTO
│   ├── logger/ tracing/     # 基础设施（结构化日志、链路追踪）
│   ├── repository/          # 实体与接口；postgres/*（含反射式行映射 mapper.go）、redis/* 实现
│   ├── scheduler/           # 进程内定时任务
│   ├── server/              # 路由与中间件
│   └── service/             # 业务逻辑
│       ├── syncer/          # 外部用户数据源连接器
│       ├── notify/          # 通知发送器（SMTP / 阿里云短信 / 通用 HTTP 短信）
│       └── storage/         # 存储后端（本地磁盘 / S3·MinIO）
├── pkg/                     # dbclient / redisclient / migrator
├── migrations/              # NNNN_name.up.sql + .down.sql
├── configs/                 # 配置占位符
├── scripts/
│   ├── smoke-test.sh        # 端到端冒烟测试
│   └── screenshot.mjs       # 无头浏览器截图（开发自检）
└── frontend/web/            # React 控制台
    └── public/icon.svg      # 系统图标（矢量）
```

## 默认端口

| 服务 | 端口 |
|------|------|
| 后端 API / OIDC | 8100 |
| 前端开发服务器 | 5173 |
| PostgreSQL | 5432 |
| Redis | 6379 |

## 许可

内部项目，未发布许可证。
