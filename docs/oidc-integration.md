# OIDC 接入指南

> 面向第三方业务系统开发者：如何把业务系统接成 Polaris 的 OIDC 客户端（RP）。
> 依据：`docs/specs/2610101436-tenant-rearchitecture-and-maxkey-borrowing.md` §3.5。

Polaris 作为 **OIDC Provider（IdP）**，对外提供标准的 OpenID Connect 授权码流程。
业务系统只需实现标准的 OIDC 客户端即可完成统一登录。

---

## 1. 端点（Discovery）

所有端点由发现文档给出，客户端应优先读取发现文档而非硬编码：

```bash
curl http://<polaris-host>/.well-known/openid-configuration
```

| 端点 | 路径 | 说明 |
|------|------|------|
| Discovery | `/.well-known/openid-configuration` | 发现文档 |
| JWKS | `/.well-known/jwks` | 公钥集（id_token 验签） |
| Authorization | `/oauth/authorize` | 授权端点（要求本地登录态） |
| Token | `/oauth/token` | 令牌端点 |
| UserInfo | `/oauth/userinfo` | 用户信息（GET / POST） |
| Introspect | `/oauth/introspect` | 令牌内省 |
| End Session | `/oauth/logout` | 结束会话 |

`issuer` 由部署配置决定（`auth.oidc.issuer`，回退到 `auth.oauth.base_url`），客户端应校验 id_token 的 `iss` 与该值一致。

请求租户解析（多租户）：Polaris 按请求 `Host` 匹配 `institutions.domain` 决定租户；未命中回退默认租户（`auth.oauth.default_institution`，默认 `built-in`）。因此**不同租户各用独立域名**。

---

## 2. 在 Polaris 中创建应用

在控制台「应用」页创建一个应用（租户内），配置：

- **Client ID / Client Secret**：创建后自动生成（`client_id` 展示于表单，`client_secret` 可一键复制）。
- **Redirect URIs**：允许的回跳地址；支持精确匹配与 `https://host/*` 前缀通配。**必须与客户端提交的 `redirect_uri` 完全一致（或前缀匹配）**。
- **Grant Types**：勾选 `authorization_code`（浏览器登录）与 `refresh_token`。
- **Scopes**：至少 `openid`；按需勾选 `profile` / `email` / `phone`。未配置 scopes 的应用不限制。
- **Cert**（可选）：id_token 签名证书；不选则用内置签名证书。

对应 REST API：`POST /api/v1/applications`（见《API 参考》）。

---

## 3. 授权码流程（Authorization Code + PKCE）

### 3.1 发起授权

将用户浏览器重定向到授权端点：

```
GET /oauth/authorize
  ?client_id=<client_id>
  &redirect_uri=<registered_redirect_uri>
  &response_type=code
  &scope=openid profile email
  &state=<opaque_csrf_state>
  &nonce=<opaque_nonce>
  &code_challenge=<PKCE_S256_challenge>       # 推荐
  &code_challenge_method=S256                 # 或 plain
```

- `response_type` 目前仅支持 `code`。
- `scope` 必须落在应用允许列表内（未配置 scopes 的应用不限制）；`openid` 恒定允许。
- 若用户未登录，`/oauth/authorize` 会返回 `UNAUTHORIZED`（401）——客户端应先引导用户在 Polaris 登录后再发起授权。
- 校验通过后，Polaris 返回 `302` 重定向到 `redirect_uri?code=<code>&state=<state>`。

### 3.2 换取令牌

```
POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&code=<code>
&redirect_uri=<same_redirect_uri>
&client_id=<client_id>
&client_secret=<client_secret>
&code_verifier=<PKCE_verifier>
```

客户端凭证可用 HTTP Basic 头（`client_secret_basic`）或表单字段（`client_secret_post`）。

响应：

```json
{
  "access_token": "…",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "…",
  "id_token": "…",
  "scope": "openid profile email"
}
```

授权码**一次性消费**（重放返回 `invalid_grant`、`reason=code_replayed`），并校验 PKCE（`S256` 与 `plain`）。

### 3.3 获取用户信息

```
GET /oauth/userinfo
Authorization: Bearer <access_token>
```

返回按 scope 收敛的 claims（见下）。

### 3.4 刷新令牌

```
POST /oauth/token
grant_type=refresh_token&refresh_token=<token>&client_id=…&client_secret=…
```

默认**轮换**：旧 `refresh_token` 立即失效；若应用开启了 `disable_refresh_rotation`，则复用原 refresh_token，仅重签 access_token。

---

## 4. Claims 与 Scope

| scope | 输出的 claims |
|-------|----------------|
| （恒有） | `sub`、`iss`、`aud`、`exp`、`iat`、`auth_time`、`preferred_username`、`institution`、`is_admin`（id_token）；`sub`（userinfo） |
| `profile` | `name`、`picture`、`locale`、`roles`、`groups` |
| `email` | `email`、`email_verified` |
| `phone` | `phone_number` |
| `address` | （保留） |
| `offline_access` | 不改变 claims；**控制是否下发 refresh_token** |

> **`offline_access` 语义**：仅在请求 scope 含 `offline_access` **且**应用允许刷新（`refresh_expire_in_hours > 0`）时，
> 令牌响应才包含 `refresh_token`。未申请 `offline_access` 的授权码/密码流程不会得到 refresh_token。

### 4.1 应用适配器（claim 映射）

应用的 `adapter` 决定 userinfo / id_token 的 claim 映射（Spec §3.5(a)、D6）：

- `adapter = "default"`：标准 OIDC claims（上表行为）。
- `adapter = "custom"`：以 `adapter_config.claimMap`（`{"<claim>": "<userAttr>"}`）覆盖/新增 claim。

可映射的用户属性（`userAttr`）白名单：`name` `username` `uid` `display_name` `email` `email_verified` `phone` `phone_verified` `avatar` `picture` `language` `locale` `institution` `org` `is_admin` `type` `first_name` `last_name` `title` `affiliation` `homepage` `location` `address` `country_code` `gender` `birthday` `education` `bio` `signup_application`。

示例：把邮箱 claim 换成手机号，并新增 `employeeNo`：

```json
{ "claimMap": { "email": "phone", "employeeNo": "uid" } }
```

规则：空值（空字符串 / `false`）不写入（不覆盖既有 claim）；未知属性名忽略。配置即生效（无需重启）。

---

## 5. 前后端登出

### 5.1 前端登出（RP 发起）

重定向到结束会话端点：

```
GET /oauth/logout            # 或 POST
```

Polaris 清除本地会话与 Cookie。

### 5.2 后端登出（Back-Channel Logout）

当应用配置了 `backchannel_logout_uri` 时，用户在任何入口登出 Polaris 后，Polaris 会向该地址
`POST`（`application/x-www-form-urlencoded`）发送 `logout_token`：

```
POST <backchannel_logout_uri>
Content-Type: application/x-www-form-urlencoded

logout_token=<JWT>
```

`logout_token` 为 RS256 签名的 JWT，JOSE 头 `typ=logout+jwt`，claims 含 `iss`/`aud`（=client_id）/
`iat`/`exp`/`jti`/`events`（`http://schemas.openid.net/event/backchannel-logout`），并带 `sub` 与 `sid`。

RP 校验要点：
1. 用 Polaris JWKS（`kid` 匹配）验签，且 `typ` 必须为 `logout+jwt`；
2. 校验 `iss` 与 `aud`；
3. 校验含 backchannel-logout 事件、且**不含** `nonce`；
4. 按 `sub`/`sid` 注销 RP 本地会话。

发送失败不阻断 Polaris 登出，但会写入审计（`oauth.backchannel_logout`）。

---

## 6. 密钥与验签（JWKS 多密钥轮换）

- id_token / logout_token 用 RS256 签名，JOSE 头带 `kid`。
- 发现文档声明 `backchannel_logout_supported=true`、`backchannel_logout_session_supported=true`。
- **多密钥并存**：`/.well-known/jwks` 同时发布激活密钥与历史（附加）密钥；客户端应缓存 JWKS 并按令牌 `kid` 选用对应公钥，切勿只认第一把钥匙。
- 轮换：新增证书 → 重启后新密钥成为激活密钥并进入 JWKS → 旧令牌在其有效期内仍可用旧密钥验签 → 旧令牌全部过期后移除旧证书。

---

## 7. 常见错误

| 错误码 | HTTP | 含义 |
|--------|------|------|
| `OAUTH_INVALID_CLIENT` | 401 | client_id/secret 错误或应用被禁用 |
| `OAUTH_INVALID_REDIRECT` | 400 | redirect_uri 未注册 |
| `OAUTH_INVALID_SCOPE` | 400 | scope 超出应用允许列表 |
| `OAUTH_INVALID_GRANT` | 400 | 授权码/刷新令牌无效、已用或过期（`reason` 给出细分） |
| `OAUTH_UNSUPPORTED_GRANT` | 400 | grant_type 不支持 |
| `UNAUTHORIZED` | 401 | 未登录就调用 `/oauth/authorize` |

响应统一为 `{ "code": "...", "message": "...", "details": {...} }`。

---

## 8. 最小流程清单

1. `GET /.well-known/openid-configuration` → 缓存端点与 `issuer`；
2. 生成 `state`/`nonce`/PKCE，重定向 `/oauth/authorize`；
3. 回调拿 `code`，校验 `state`；
4. `POST /oauth/token` 换 `id_token`/`access_token`；
5. `GET /.well-known/jwks` 取公钥，按 `kid` 验签 id_token，校验 `iss`/`aud`/`exp`/`nonce`；
6. `GET /oauth/userinfo` 取用户信息；
7. （可选）配置 `backchannel_logout_uri` 接收登出通知；
8. 需要离线访问时用 `refresh_token` 轮换。
