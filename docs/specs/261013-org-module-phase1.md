# 组织管理模块优化 · 第一阶段 Spec（密码策略深度 + Logo + 存储）

> 状态：**已实现（第 1/2/3/4/5 步全部落地并测试）** · 创建：2026-10-13 · 更新：2026-10-13 · 依据：用户反馈 + [Casdoor](https://github.com/casdoor/casdoor) 对标
> 范围：组织字段与密码策略、平台/组织 Logo 的「上传或 URL」、系统默认对象存储（S3）
> 目标：让组织管理可用于生产——「配置即生效」，不留无效字段。

---

## 1. 背景

组织（institution）是多租户隔离边界与资源归属维度。当前 `institutions` 表字段多来自 Casdoor 平移，
存在三类问题（均已核实）：

1. **配了不生效**：`password_type` 只入库、从不参与哈希（永远 bcrypt）；无组织级 `password_salt`；
   `password_options` 前端为逗号文本框、后端以英文模糊匹配解析，种子数据与 Casdoor 枚举码不一致。
2. **无上传/URL 双模**：Logo 无上传入口，也没有「上传或填 URL」的统一字段组件；平台 Logo（登录页/侧边栏/Portal）
   未接入组织 Logo。
3. **无系统默认存储**：已有 `providers(category=Storage)` + 完整 S3/MinIO/local 后端，但**仅组织级**且只用于头像，
   缺少「系统默认存储」与通用上传接口。

## 2. 决策（已定稿）

- **组织标识**：复用现有 `name` 作为「组织标识（Code）」（URL/外键/登录不变），`display_name` 作为「名称」。**不新增字段**。
- **密码类型**：**全量落地**并做好抽象（见 §3），7 类算法 + 固定盐算法，真正接入登录/改密/注册/重置链路。
- **密码盐**：组织级 `password_salt`，**write-only**（响应只回 `has_password_salt`，不回明文）。
- **强度策略**：改枚举码多选（`AtLeast6/AtLeast8/Aa123/SpecialChar/NoRepeat`），并兼容历史自由文本。
- **存储归属**：平台级默认存储 + 组织可覆盖（组织配了 `Storage` 提供商则优先，否则回落默认）。
- **字段处理**：第一阶段只做「加法 + 接线」，**不删除**旧字段；删除清单单列（见 §6）。

## 3. 已实现（第 1 步）：密码凭据抽象与组织密码策略

### 3.1 `internal/auth/cred` 抽象

单一接口 + 注册表，新增算法零改动调用方：

```go
type Manager interface {
    Type() string
    Hash(password, salt string) (string, error)
    Verify(password, hash, salt string) bool
}
func Get(passwordType string) Manager   // 未知类型回退 bcrypt（避免配置异常锁死登录）
func SupportedTypes() []string
```

实现（对齐 Casdoor `cred` 包，保证算法兼容）：

| password_type | 说明 | 盐语义 |
|---|---|---|
| `bcrypt` | 默认 | 哈希自带随机盐，忽略 salt |
| `argon2id` | PHC 编码 `$argon2id$v=19$m=..,t=..,p=..$salt$hash` | 自带随机盐，忽略 salt |
| `salt` | SHA-256 加盐（Casdoor 的 `salt`） | 固定盐参与运算 |
| `sha512-salt` | SHA-512 加盐 | 固定盐参与运算 |
| `md5-salt` | MD5 加盐（兼容旧数据，不推荐） | 固定盐参与运算 |
| `pbkdf2-salt` | PBKDF2-SHA256（27500 迭代，Keycloak 约定） | base64 解码参与；非法 base64 回退原始字节 |
| `pbkdf2-django` | 自描述 `pbkdf2_sha256$iters$salt$hash` | 忽略外部 salt |
| `plain` | 明文（仅用于兼容外部导入，默认不启用） | 忽略 |

### 3.2 `internal/auth/password.go` 门面

- `HashPasswordWith(password, passwordType, salt)` / `VerifyPasswordWith(password, hash, passwordType, salt)`；
- 保留 `HashPassword` / `VerifyPassword`（bcrypt 默认）向后兼容；
- `FromOptions` 支持枚举码 + 历史自由文本；`Validate` 新增 `NoRepeat`。

### 3.3 接线点（「配置即生效」）

哈希使用**组织的** `password_type` + `password_salt`，并把结果类型/盐落到用户记录；
校验使用**用户存储的**类型/盐（历史空值回退 bcrypt）：

- `service/user_service.go`：创建用户、默认密码、管理员重置、自助改密、密码历史去重；
- `service/auth_service.go`：密码登录校验；
- `service/oidc_service.go`：password 授权类型校验；
- `app/seed.go`：内置组织/管理员。

### 3.4 数据与接口

- 迁移 `008_institution_password_salt`：新增 `institutions.password_salt`；归一化 `password_options` 为枚举码。
- 仓库 `institution_repo.go`：列清单 / scan / insert / update 同步 `password_salt`。
- DTO `institutionDTO`：`has_password_salt`（不暴露明文）。
- 服务 `institution_service.go`：校验密码类型与强度策略为受支持取值；更新时空盐保留既有值。
- 前端：`password-policy.ts`（单一来源）；组织新建/编辑弹窗与组织详情设置卡新增
  密码类型下拉、强度多选、Salt（write-only）、过期天数；中英词条同步。

### 3.5 测试

- `internal/auth/cred/cred_test.go`：全类型往返、空密码、未知类型回退、随机盐唯一性、
  固定盐差异、django 自描述。
- `internal/auth/password_test.go`：枚举强度策略、按类型哈希/校验、未知类型回退。
- `internal/handler/dto_test.go`：组织响应**绝不**含 `password_salt` 明文。

## 4. 已实现（第 2 步）：Logo「上传或 URL」

- 后端：通用上传端点 `POST /api/v1/upload`（受保护；分类白名单 `logos/favicons/avatars/application/images`；≤5MB；键 `<category>/<institution>/<uuid><ext>`）。
- 前端：`shared/components/upload-or-url.tsx`——单字段二选一（填链接 / 上传文件），左侧常驻预览、支持点击与拖拽投放、上传态与统一错误提示；接入组织 Icon 与 Favicon。
- 平台 Logo：`BrandMark` 支持 `src`（`<img>` 回退内置矢量），顶栏与用户门户使用「当前用户所属组织的 Logo」；浏览器 Favicon 亦随当前组织切换（React 19 `<link>` 提升）。
- 登录页品牌取自**组织**：左上角显示组织名（含组织 Logo），欢迎语使用**组织描述**（不再回退到平台名）；
  真实登录页与组织编辑页的实时预览口径一致（`/auth/app-info` 下发 `org_display_name` / `org_description` / `org_logo`）。

## 5. 已实现（第 3 步）：系统默认对象存储（S3）

- 迁移 `009_storage_settings`：单行 `storage_settings`（`type/endpoint/region/bucket/access_key/secret_key_enc/path_prefix/domain/root_path/public_base/path_style`）。
- 平台级默认存储由控制台配置（`GET/PUT /api/v1/system/storage`，`POST .../test` 连通性自检），Secret Key 经 `SecretCipher` 加密落库、**write-only**（仅回 `has_secret`）。
- `StorageService` 选择顺序：组织的 `Storage` 提供商（`state=active`）→ 平台默认 → `PROVIDER_UNAVAILABLE`；本地文件解析同样支持两级回落。
- 前端：新增平台导航「系统设置 › 存储设置」（`/settings/storage`），按类型（local / S3 / MinIO）动态呈现字段。

## 6. 已实现（第 4 步）：精简只写不读的冗余字段

迁移 `010_trim_institution_fields` 删除 7 个"仅入库/回显、运行时从不读取"的组织字段：
`languages`、`ip_whitelist`、`mfa_items`、`enable_soft_deletion`、`is_profile_public`、
`master_password_hash`（连同只写不用的 `UpdateMasterPassword` 与 DTO `has_master_password`）、`theme_data`（组织级；
应用级 `theme_data` 保留）。同步更新模型 / 仓库列清单 / DTO / 前端类型 / 种子数据。

**修正**：`default_password` 经核查在"新建用户无密码"时被读取（`user_service`），**保留**。

**另发现的死字段（本轮未动，待确认）**：`captcha`（实际走 providers 的人机校验）、
`token_retention_days`、`record_retention_days`、`max_sessions`、`enable_exclusive_signin`
（后两者在应用级另有生效实现）。如确认无用，可再走一次精简迁移。

## 7. 影响面与风险

- 认证核心链路（登录/改密/重置）改动最广，已用测试覆盖；默认仍是 bcrypt，历史数据不受影响。
- `password_options` 归一化为不可逆（原自由文本丢失），已在 down 迁移注明。

## 8. 已实现（第 5 步）：组织级 MFA 策略

对齐 Casdoor 的「组织 MFA 设置项 + 记住时间」，但**必须真生效**（此前的 `mfa_items` 因"只存不读"被删）。

- **数据**：迁移 `011_institution_mfa_policy` —— `mfa_policy`（`optional` 默认 / `required`）+
  `mfa_remember_hours`（0 = 不记住），带 CHECK 约束；仓库/DTO/服务校验同步。
- **登录判定**（`auth_service`）：
  - 已绑定 MFA：登录必须通过二次校验；命中有效的「信任设备」令牌可跳过；
  - 组织 `required` 且未绑定：登录成功后会话被标记 `mfa_enroll_required`，返回 `mfa.enroll=true`；
  - 密码登录 / 验证码登录 / SSO 回调**三条入口一致**（均在会话上标记，避免绕过）。
- **硬门禁**：`auth` 中间件对待绑定会话只放行 `/auth/mfa/setup|verify`、`/auth/logout`、`/auth/status`、
  `/me`，其余返回 `MFA_ENROLL_REQUIRED`；绑定完成即清除标记。
- **记住设备**：校验通过且勾选时，签发独立的 `MfaTrustClaims` JWT 写入 HttpOnly Cookie
  `polaris_mfa_trust`（与会话令牌声明结构分离，不可互相冒用）。
- **前端**：组织编辑「密码与安全」新增 MFA 分区；登录页支持「强制绑定」步骤与「记住此设备」勾选。
- **测试**：策略校验、信任令牌往返/不匹配、门禁白名单、DTO 下发。

**未纳入**：组织级 LDAP（同步能力已由「外部数据源」承载；LDAP 密码登录与 Kerberos/Radius 因定位冲突不实现）。
