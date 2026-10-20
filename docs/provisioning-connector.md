# 账号供给接入指南（Connector 契约）

> 面向下游应用：如何实现一个 Polaris **下游连接器（Connector）**，接收"推送型"账号供给与生命周期事件。
> 依据：`docs/specs/2610101436-tenant-rearchitecture-and-maxkey-borrowing.md` §3.3。

Polaris 在本地用户/角色/登录/离职等事件发生时，按**账号策略**把账号**推送**到下游应用。
下游只需实现下述 RESTful 契约即可接入。

---

## 1. 契约总览

连接器以一个 **base URL**（`connector.provider_url`）为根，鉴权用 **HTTP Basic**（`principal:credentials`，凭证 AES-GCM 加密存储）：

| 动作 | 方法 | 路径 | 语义 |
|------|------|------|------|
| 建号 | `POST` | `{base}/accounts` | 创建下游账号 |
| 更新/改密 | `PUT` | `{base}/accounts/{username}` | 更新属性或改密 |
| 启用 | `POST` | `{base}/accounts/{username}/enable` | 启用账号 |
| 禁用 | `POST` | `{base}/accounts/{username}/disable` | 禁用账号 |
| 删除 | `DELETE` | `{base}/accounts/{username}` | 删除账号 |
| 自检 | `GET` | `{base}/health` | 连通性自检 |
| 查询账号 | `GET` | `{base}/accounts/{username}` | **对齐检查**用：`2xx` = 存在，`404` = 不存在，其余状态码视为"无法判定" |

请求体为 `application/json`（建号/更新时）：

```json
{
  "username": "alice",
  "display_name": "Alice",
  "email": "alice@example.com",
  "phone": "13800000000",
  "password": "…",
  "attributes": { "title": "Engineer" }
}
```

- `username`：期望的下游账号名（来自策略 `mapping`，见 §3）。
- `password`：仅在建号时按需下发（用于下游初始口令）；更新时的改密通过 `PUT` 携带。
- `attributes`：按 `mapping` 生成的附加属性（本地属性 → 下游属性）。

**建号响应**：可返回 `{"username": "<下游实际账号名>"}`；未返回则回退为请求中的 `username`。
该下游账号名会记录到 `app_accounts.related_username`。

成功判定：HTTP 2xx。非 2xx 视为失败并记录到供给历史（`provision_history`），可按策略重试。

---

## 2. 幂等性

- 幂等键：`(institution, application, user)`（`app_accounts` 唯一约束）。
- 建号应**幂等**：对同一 `username` 重复 `POST` 应返回成功（已存在则返回既有账号），避免重试造成重复建号。
- 禁用/启用/删除对已处于目标状态的账号应为无操作成功（幂等）。

---

## 3. 账号策略（AccountStrategy）

策略把"何时建号 / 何时回收"和"本地属性 → 下游属性"配置化：

- `mapping`：`{"<本地属性>": "<下游属性>"}`。可用本地属性：
  `name`/`username`、`displayName`/`display_name`、`email`、`phone`、`uid`、`title`、`location`。
- `policy.createWhen`：`manual`（仅手动）/ `onCreate`（用户创建即建号，默认）/ `onGrant`（角色授予即建号）/ `jit`（首次登录即时建号）。
- `policy.revokeWhen`：`disable`（用户禁用即禁用下游账号，默认）/ `delete`（删除即删除）/ `offboard`（离职即删除）。

控制台在「应用详情 › 账号供给 › 策略」按应用可视化配置；REST：`/api/v1/account-strategies?application_id=<id>`。

---

## 4. 触发点（何时发生）

Polaris 在以下事件接线触发供给（配置即生效）：

1. **用户创建** → 命中 `onCreate` 策略的应用建号；
2. **角色授予** → 命中 `onGrant` 策略的应用建号；
3. **首次登录** → 命中账号策略 `createWhen=jit` 的应用建号；
4. **用户禁用 / 离职（软删除）/ 删除** → 按 `revokeWhen` 禁用或删除下游账号。

执行模型：asyncd 异步任务 + scheduler 兜底重试；下游失败不影响本地主流程。全过程写入 `provision_history`（动作 / 状态 / 消息 / 时间），在「应用详情 › 账号供给 › 同步记录」查看；应用账号可在同一应用的「应用账号」子页手动建号 / 重试；删除会先派发下游 `delete` 任务，成功后才删除本地台账。出方向连接卡片也可按连接器查看该连接器的同步记录。

> 多实例部署下进程内 scheduler 会重复触发（已知限制，后续引入 PG advisory lock）。

---

## 5. 连接器配置项

在控制台「身份连接 → 出方向」创建（REST `/api/v1/connectors`）：

| 字段 | 说明 |
|------|------|
| `application` | 关联应用标识（`code`）；**一个应用只能绑定一个连接器**（DB 唯一索引保证） |
| `type` | 连接器类型：`http`（通用 RESTful 契约）/ `dingtalk` / `lark`（飞书与 Lark）/ `wecom` / `ldap`（AD 与 OpenLDAP） |
| `config` | **类型专属的非敏感配置**（键由类型目录声明）：通用 HTTP 为 `provider_url` / `principal`；钉钉为 `app_id` / `dept_id`；飞书为 `app_id` / `dept_id` / `base_url`；企业微信为 `app_id` / `dept_id`；目录服务为 `host` / `port` / `ssl_mode` / `user` / `base_dn` / `rdn_attr` / `directory_type` |
| `secrets` | **类型专属的敏感值（明文提交，加密落库）**：`credentials`（通用 HTTP）/ `app_secret`（钉钉·飞书·企业微信）/ `password`（目录服务的绑定口令）。留空表示不修改 |
| `state` | `active` / `disabled` |

> 未声明的键会被拒绝（避免"前端多传一个键、后端悄悄存下却没人读"）；**敏感键必须经 `secrets` 提交**，
> 混进 `config` 会被直接拒绝。响应只下发 `config`（非敏感）与 `has_secrets`（各敏感键是否已配置），
> 绝不下发密钥明文。各类型的字段清单由 `GET /api/v1/connections/catalog` 下发（前端据此渲染表单）。

**连接器能力（Capabilities）决定哪些动作可用**——能力可能依赖配置，例如同为目录服务连接器，
Active Directory 支持"停用账号"（`userAccountControl`），OpenLDAP 没有等价语义：

| 类型 | 口令 | 更新 | 停用 | 删除 |
|---|---|---|---|---|---|
| 通用 HTTP | ✅ | ✅ | ✅ | ✅ |
| 钉钉 | ❌（SSO 场景） | ✅ | ❌（服务端 API 无停用字段；离职走智能人事，本阶段不接） | ✅ |
| 飞书 / Lark | ❌ | ✅ | ❌（contact v3 无停用态，仅删除与恢复） | ✅ |
| 企业微信 | ❌ | ✅ | **✅**（`user/update` 的 `enable` 字段：1 启用 / 0 禁用） | ✅（官方标注需"通讯录同步助手或第三方通讯录应用"权限） |
| 目录服务（OpenLDAP） | ✅ | ✅ | ❌ | ✅ |
| 目录服务（AD） | ✅ | ✅ | ✅ | ✅ |

### 文档核对记录（2026-10-13）

对接依据与核对结果（**只写实际核对过的结论**）：

| 厂商 | 核对来源 | 结论 |
|---|---|---|
| 企业微信 | 官方文档「创建成员 / 更新成员 / 读取成员 / 删除成员」+ 全局错误码 | 路径与参数一致；**发现并修正**：`enable` 字段（0 禁用 / 1 启用）确实存在 → 企微由"不支持停用"改为**支持停用**；错误码 `60111 = UserID不存在` 与实现一致；删除成员官方标注权限限制 |
| 飞书 / Lark | 官方 Go SDK（`larksuite/oapi-sdk-go`）的 contact v3 与 auth v3 资源定义、`User` 模型 | 路径 `/open-apis/contact/v3/users[/:user_id]`、鉴权端点 `/open-apis/auth/v3/tenant_access_token/internal`、查询参数 `user_id_type=user_id`、请求体即 `User` 对象（`user_id`/`name`/`mobile`/`email`/`department_ids`/`job_title`/`city`/`employee_no`）全部与实现一致 |
| 钉钉 | 官方新版 Java SDK（`com.aliyun:dingtalk`）全量 API 清单 | contact/hrm 下**不存在"创建员工"接口**（只有离职/终止类流程），因此创建走 服务端 API `topapi/v2/user/create` 是正确选择（与入方向同源）。**待真实环境确认**：`user/get` 的"用户不存在"错误码按 `60121` 映射——映射错误只会得到 `无法判定`（不会误建号），真实环境一跑即可校准 |
| 目录服务 | Microsoft/OpenLDAP 通行约定 | AD 口令走 `unicodePwd`（UTF-16LE 且必须加密连接）、停用走 `userAccountControl` 的 ACCOUNTDISABLE 位；OpenLDAP 用 `userPassword` 且无停用语义 |

不支持的动作由服务层**提前拒绝**，并在供给历史里以 `skipped` 如实记录（既不假装成功，也不无限重试）。

「测试连通性」按钮调用 `GET {base}/health` 自检。

---

## 6. 实现要点清单

1. `GET {base}/health` 返回 2xx 即视为可用；
2. 校验 HTTP Basic 凭证；
3. `POST /accounts` **幂等**建号，返回 `{"username": …}`；
4. `PUT /accounts/{username}` 支持属性更新与改密；
5. `POST /accounts/{username}/enable|disable` 与 `DELETE /accounts/{username}` 幂等；
6. 返回标准 JSON；错误用非 2xx + 可读消息（消息会进入供给历史，便于排障）；
7. 大流量下建议对 `username` 建唯一索引以保证幂等。
