# Polaris IAM 平台设计说明

> 状态：已实现（v0.1） · 最后更新：2026-10-09

## 1. 背景与目标

为满足"统一登录 + 统一授权 + 组织级多租户"的诉求，建设自研身份认证平台。能力面参照 Casdoor，
工程规范参照 SkyPivot（模块化单体、Handler→Service→Repository、统一错误码与响应、迁移驱动）。

**目标**

1. 单一身份源：用户、组织、应用集中管理，各业务系统通过 OIDC 接入。
2. 组织隔离：以组织作为租户边界，用户、应用、角色、权限均在组织内隔离。
3. 可扩展接入：既作为 IdP 对外，也能作为 SP 接入钉钉、微信、飞书、通用 OIDC。
4. 可运维：审计可追溯、会话可强制下线、密钥可轮换。

**非目标**

- 不做商业计费能力（产品、订单、支付、定价、套餐、优惠券、订阅、交易、余额）。
- 不做 Casdoor 管理工具中的「表单 Forms」（列表页列配置模板）与「工单 Tickets」（客服工单）。
- 暂不做 Webhooks / Webhook Events；LDAP 与 SCIM 当前为"客户端连接器"方向（从外部拉取用户），不提供目录服务端。

## 2. 领域模型

以 `organization` 为租户维度，业务表均对组织建立外键并 `ON DELETE CASCADE`。

```
Organization --< User --< UserIdentity（provider + provider_user_id 唯一）
      |             |
      |             +--< PasswordHistory（组织密码策略：禁止复用最近 N 次）
      +--< Application（OAuth2/OIDC 客户端：client_id / client_secret / redirect_uris / grant_types）
      +--< Provider（外部身份源：dingtalk / wechat / feishu / oidc）
      +--< Group --< GroupMember
      +--< Role  --< RoleUser / RoleGroup / RoleRole（嵌套） / RolePermission
      +--< Permission --< PermissionUser / PermissionGroup / PermissionRole
      +--< Cert（RSA 自签名，用于 id_token 签名）
      +--< Token（授权码 / 访问令牌 / 刷新令牌 / id_token 记录，用于内省与吊销）
      +--< Syncer --< SyncerRun（外部数据源与执行记录）
      +--< Record（审计）
```

**关键唯一约束**

| 约束 | 说明 |
|------|------|
| organizations.name | 组织名唯一 |
| users (organization, name) | 组织内账号唯一 |
| users (organization, lower(email))，email 非空 | 组织内邮箱唯一，防止跨身份源重复建号 |
| users.uid，非空 | 对外主体标识（OIDC sub），与可改的 name 解耦 |
| applications.client_id | 客户端 ID 全局唯一 |
| user_identities (provider, provider_user_id) | 同一外部身份只能绑定一个本地用户 |
| roles / permissions / groups 的 (organization, name) | 组织内命名唯一 |

## 3. 认证与授权

### 3.1 会话模型

- Redis 键 `polaris:session:<uuid>` 存 JSON 会话（用户、组织、角色、**有效权限规则**、IP/UA、过期时间）。
- 用户会话索引 `polaris:user_sessions:<org>:<user>`（Set），支撑"强制下线"与"权限变更即失效"。
- JWT 仅含 `sid`（session id）与标准声明；Cookie 名 `polaris_session`，HttpOnly，生产 Secure。
- 中间件解析顺序：`Authorization: Bearer`（先按会话 JWT 解析，失败回退 API access token）→ Cookie。

### 3.2 权限模型

权限是「资源 × 动作 × 效果」的集合，主体通过三种途径获得：

1. 直接绑定（permission_users）；
2. 经用户组（permission_groups）；
3. 经角色（role_permissions）；角色可嵌套（role_roles 递归），角色也可挂用户组（role_groups）。

判定顺序：**管理员直通 → Deny → Allow**；resource / action 支持 `*` 通配。
前后端语义完全一致（后端 `internal/auth/model.go`，前端 `src/features/auth/auth-context.tsx`），由单测锁定。

### 3.3 账号供给（SSO）

按 Provider 配置：mode（jit / email_match / prebound）、auto_create、link_by_email、allowed_domains、default_role。
查找顺序：

```
1. 已绑定身份（provider + provider_user_id）命中 -> 直接返回
2. 邮箱匹配（link_by_email 且命中组织内用户）-> 建绑定后返回
3. auto_create -> JIT 建号并绑定（用户名自 username / email 派生，冲突时追加随机后缀）
   否则返回 IDENTITY_NOT_BOUND
```

## 3.4 应用（Application）策略

应用的每个字段都对应一个运行时读取点（"配置即生效"）：

| 配置 | 生效位置 | 行为 |
|------|----------|------|
| `disable_signin` / `state` | 登录、发码、授权 | 直接拒绝（403），应用不可用 |
| `enable_password` | 密码登录 / password grant | 关闭即拒绝密码登录 |
| `enable_sign_up` / `disable_self_signup` | 注册、验证码登录 JIT 建号 | 关闭即拒绝自助建号 |
| `ip_whitelist` / `ip_restriction` | 登录、发码 | 支持单 IP 与 CIDR；不命中即 403 |
| `max_sessions` / `enable_exclusive_signin` | 登录成功后 | 超出并发数踢最旧会话；独占则踢其他全部 |
| `enable_code_signin` | 发码 / 验证码登录 | 关闭即拒绝（403） |
| `providers[]` | 登录页 | 仅返回 `canSignIn` 的提供方（应用级白名单） |
| `scopes[]` | authorize / token | 请求 scope 超出应用许可即 `OAUTH_INVALID_SCOPE` |
| `grant_types[]` | token | 未声明的 grant 直接拒绝 |
| `expire_in_hours` / `refresh_expire_in_hours` | 签发 | 覆盖签发器默认有效期 |
| `disable_refresh_rotation` | refresh_token | 关闭轮换时保留原 refresh_token 并标记复用 |
| `token_fields[]` | access_token | 白名单裁剪内嵌声明（空 = 仅 username/org） |
| `cookie_expire_in_hours` | 会话 Cookie | 覆盖全局会话 TTL |
| `theme_data` / `logo` / `favicon` / `title` | 登录页（app-info） | 应用级品牌覆盖 |
| `signup_items[]` / `signin_items[]` / `signin_methods[]` | 登录页 | 下发给前端用于字段编排与校验 |
| `signup_url` / `signin_url` / `forget_url` / `terms_of_use` | 登录页 | 跳转地址 |

同时未接入运行时的 Casdoor 字段（SAML 系列、WebAuthn、FederatedCredentials、自定义 HTML/CSS）**不做字段落库**，
避免出现"后台可配置但没有任何效果"的死配置。

## 3.5 提供商（Provider）多类别

| 类别 | 子类型 | 运行时能力 | 自检动作 |
|------|--------|-----------|----------|
| OAuth | dingtalk / wechat / feishu / lark | 授权码登录、JIT 建号 | 校验 client_id/secret 完整性 |
| OIDC | oidc | 标准 discovery + 授权码 | 校验 issuer 完整性 |
| Email | smtp | 发送验证码/通知（`net/smtp`，支持 STARTTLS） | 建立连接并完成认证 |
| SMS | aliyun / http | 发送验证码（阿里云 RPC HMAC-SHA1 签名 / 自定义 HTTP 网关） | 配置了 receiver 时真实发送 |
| Captcha | image / hcaptcha / recaptcha / turnstile | 登录与发码前的人机校验（本地图形答案存 Redis，多实例一致） | 本地生成一次 / 远程校验密钥完整性 |
| Storage | local / s3 / minio | 头像等平台托管文件的写入与直出 | 写入并删除探测对象 |

**验证码登录**：`POST /auth/codes` 发码（同目标 60s 一次、每小时上限 10 次，命中人机校验），
`POST /auth/login/code` 校验（Redis 一次性消费）后建会话；未命中用户且应用允许时按 `register_type=code` JIT 建号。
发码依赖组织内 `state=active` 的 Email/SMS 提供商，缺失时返回 `PROVIDER_UNAVAILABLE`（不静默失败）。

**头像存储**：`POST /me/avatar`（≤2MB，png/jpeg/gif/webp）写入组织内 Storage 提供商；
本地后端由 `/static/storage/*` 只读直出（键解析出组织再取该组织的本地根目录，防路径穿越），
对象存储后端返回直链或自定义域名。

## 3.6 组织为中心的视图

- 侧边栏顶部提供**组织切换器**，切换后所有资源页统一收敛到该组织（持久化到 localStorage）。
- **组织详情页**（`/organizations/:name`）给出该组织的资源规模（用户/用户组/角色/权限/应用/提供方/数据源/令牌）
  与一键进入管理的入口，同时可编辑组织设置；进入时会把当前组织切到目标组织，因此复用同一套资源页。
- **控制台**：KPI 按当前组织统计，并附"按组织聚合"表格（各组织资源规模 + 一键切换）。

## 4. OIDC Provider 实现要点

| 环节 | 处理 |
|------|------|
| 签名密钥 | 启动时加载 certs 中 `auth.oidc.cert_name` 指定证书；缺失则生成自签名 RSA 并落库 |
| JWKS | kid 取 RSA 公钥的 RFC 7638 指纹（前 16 字节 base64url），n / e 按 JWK 规范编码 |
| authorize | 校验 client_id / redirect_uri（精确匹配，支持 `/*` 前缀）/ response_type=code；要求本地登录态 |
| 授权码 | 一次性消费（code_is_used 条件更新，防重放）；记录 redirect_uri、scope、PKCE challenge |
| PKCE | 支持 S256 与 plain；RFC 7636 示例向量已纳入单测 |
| token | 4 种 grant；refresh_token 默认轮换（旧令牌置空并标记吊销） |
| id_token | 按 scope 收敛声明：profile（name / picture / roles / groups）、email、phone |
| userinfo | 由 access_token（RS256 JWT）反查用户，按 scope 返回声明 |

## 5. 外部用户数据源（Syncer）

连接器统一产出「用户 / 用户组」中立视图，由同步服务负责写回；连接器本身无状态、可独立测试。

| 类型 | 数据来源 | 说明 |
|------|----------|------|
| dingtalk | 钉钉开放平台（gettoken + 部门/成员接口） | AppID / AppSecret |
| lark | 飞书（tenant_access_token + contact v3） | AppID / AppSecret，分页 |
| wecom | 企业微信（gettoken + 部门/成员接口） | CorpID / Secret，fetch_child=1 递归 |
| ldap | LDAP / AD（Bind + Search） | base DN 取 database，过滤条件取 table |
| database | PostgreSQL / MySQL（SELECT * + 字段映射） | user_mapping 指定本地字段到源列名 |
| scim | SCIM 2.0（/Users、/Groups） | Bearer Token，分页 |

写回策略：按派生用户名匹配 → 存在则更新差异字段（display_name / email / phone / avatar / title），
不存在则创建（register_type=syncer，register_source 为同步器名）；sync_groups 时同步用户组成员。
每次执行写入 syncer_runs 并回写 last_status / last_message / last_counts。

敏感字段（口令、应用密钥）以 AES-256-GCM 加密落库，接口只返回 has_secret / has_app_secret。

## 6. 安全设计

| 项 | 措施 |
|----|------|
| 口令 | bcrypt；组织级强度策略（长度/大小写/数字/特殊字符）与历史复用限制 |
| 会话 | Redis + TTL；权限/角色/用户组/用户状态变更即失效；支持强制下线 |
| 二次验证 | TOTP（30s / 6 位，正负 1 窗口）+ 8 个一次性恢复码；密钥与恢复码加密存储且不下发 |
| 登录防护 | 组织+账号+IP 维度的失败计数限流（Redis，超阈值拒绝，成功后清零） |
| CSRF | 写操作校验 Origin 白名单；显式携带 Authorization 的请求不适用（无环境凭证） |
| 密钥管理 | 配置仅占位符，POLARIS_* 注入；生产强制 POLARIS_CREDENTIAL_ENC_KEY |
| 输出脱敏 | 统一 DTO：密码哈希 / TOTP 密钥 / 恢复码 / client_secret / 私钥 / 令牌明文 / 同步器口令一律不下发 |
| 错误返回 | 非业务错误一律 INTERNAL_ERROR，堆栈仅进服务端日志 |
| 令牌 | 授权码一次性消费；刷新令牌轮换；定期清理过期记录 |

## 7. 已知限制与后续规划

| 项 | 现状 | 规划 |
|----|------|------|
| 调度器 | 进程内定时器（多实例会重复触发） | 引入 PostgreSQL advisory lock 或独立调度 |
| Webhooks | 未实现 | 身份事件（登录 / 建号 / 绑定）推送外部系统 |
| SAML / CAS | 未实现 | 按需扩展 Application.Type 与对应端点 |
| LDAP / SCIM 服务端 | 未实现（仅客户端连接器） | 如需对外提供目录服务再补 |
| 密钥轮换 | 支持手动生成 / 替换证书 | 增加 JWKS 多密钥并存与平滑轮换 |
| 会话视图 | 按用户查询 | 增加按组织聚合的在线会话大盘 |
| 自定义登录页 HTML/CSS | 未实现（安全考量） | 如确需，改为受限模板 + 白名单标签而非任意注入 |
| 头像裁剪 | 仅原图存储 | 增加服务端裁剪与多尺寸衍生 |
