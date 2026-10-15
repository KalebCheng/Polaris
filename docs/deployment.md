# 部署手册

> Polaris 一体镜像（后端 + 前端 SPA + nginx）的生产部署指南。

---

## 1. 交付形态

Polaris 以**单容器一体镜像**交付：镜像内含 Go 后端、前端静态资源与 nginx。
nginx 托管 SPA 并把 `/api`、`/oauth`、`/.well-known`、`/static` 反代到容器内 `127.0.0.1:8100` 的 Go 服务。

外部依赖：**PostgreSQL 15+** 与 **Redis 7+**。

镜像构建（多阶段）：`node` 构建前端 → `golang` 构建后端静态二进制 → `nginx:alpine` 运行。

---

## 2. 快速开始（docker compose）

```bash
export POLARIS_JWT_SECRET=$(openssl rand -base64 32)
export POLARIS_CREDENTIAL_ENC_KEY=$(openssl rand -base64 32)
docker compose up -d
# 访问 http://localhost:8100 （组织 built-in / admin / admin123）
```

`docker-compose.yml` 会一并拉起 PostgreSQL 与 Redis，应用镜像默认取 `POLARIS_IMAGE`（默认 `polaris/polaris:latest`）。

---

## 3. 直接用镜像运行

```bash
docker run -d --name polaris -p 8100:8100 \
  -e POLARIS_ENV=production \
  -e POLARIS_DB_HOST=your-pg-host \
  -e POLARIS_DB_PASSWORD=your-pg-password \
  -e POLARIS_REDIS_HOST=your-redis-host \
  -e POLARIS_JWT_SECRET=your-jwt-secret \
  -e POLARIS_CREDENTIAL_ENC_KEY=$(openssl rand -base64 32) \
  -e POLARIS_ISSUER=https://sso.example.com \
  -e POLARIS_CSRF_ALLOWED_ORIGINS=https://sso.example.com \
  polaris/polaris:latest
```

> 生产报名必须注入 `POLARIS_JWT_SECRET` 与 `POLARIS_CREDENTIAL_ENC_KEY`（`app.env=production` 会强制要求后者）。

---

## 4. 关键环境变量

| 变量 | 说明 | 默认 |
|------|------|------|
| `POLARIS_ENV` | 运行环境（`production` 触发生产红线） | `development` |
| `POLARIS_PORT` | 后端监听端口（容器内） | `8100` |
| `POLARIS_CONFIG` | 配置文件路径（容器内 `/app/configs/app.yaml`） | `configs/app.yaml` |
| `POLARIS_DB_HOST/PORT/USER/PASSWORD/NAME` | PostgreSQL 连接 | — |
| `POLARIS_DB_SSLMODE` | 连接 SSL 模式 | `disable` |
| `POLARIS_DB_AUTO_MIGRATE` | 启动时自动迁移 | `false` |
| `POLARIS_DB_SEED` | 启动时写入种子数据 | `false` |
| `POLARIS_MIGRATIONS_DIR` | 迁移脚本目录 | `migrations` |
| `POLARIS_REDIS_HOST/PORT/DB/PASSWORD` | Redis 连接 | — |
| `POLARIS_JWT_SECRET` | 会话 JWT 密钥（**必填**） | — |
| `POLARIS_CREDENTIAL_ENC_KEY` | 凭证 AES-GCM 加密密钥（**生产必填**；提供商密钥、同步器密钥、存储密钥、TOTP 均用它加密） | — |
| `POLARIS_CONFIG` | 配置文件路径（默认 `configs/app.yaml`） | — |
| `POLARIS_COOKIE_SECURE` | 会话 Cookie Secure 标志（HTTPS 下应为 `true`） | `false` |
| `POLARIS_CSRF_ALLOWED_ORIGINS` | 允许携带 Cookie 写操作的来源（逗号分隔，**生产必配**） | — |
| `POLARIS_ISSUER` | 对外 issuer（OIDC） | 回退 `oauth.base_url` |
| `POLARIS_DEFAULT_INSTITUTION` | 未命中域名时的默认租户 | `built-in` |
| `POLARIS_OIDC_ADDITIONAL_KEY_CERT_NAMES` | 附加校验密钥证书名（JWKS 轮换，逗号分隔） | — |
| `POLARIS_ADMIN_PASSWORD` | 种子管理员密码 | `admin123` |

完整清单见 `README.md`。

---

## 5. 数据库迁移

迁移由内置 `pkg/migrator` 驱动（每迁移独立事务、advisory lock、记录 `schema_migrations`），脚本目录 `migrations/`。

- **启动时自动迁移**：设 `POLARIS_DB_AUTO_MIGRATE=true`。
- **独立执行**（推荐生产用一次性 Job）：

```bash
docker run --rm -e POLARIS_DB_HOST=… -e POLARIS_DB_PASSWORD=… \
  polaris/polaris:latest /app/polaris -migrate         # 应用全部未执行迁移
docker run --rm … polaris/polaris:latest /app/polaris -migrate-down 1   # 回滚最近 1 步
```

迁移文件**不可变**：变更只能新增迁移，禁止修改已发布文件。

---

## 6. 反向代理与 TLS

一体镜像的 nginx 监听容器内 `:80`。生产建议在其前置一层 TLS 终结（云 LB / 独立 nginx / Traefik），并：

- 透传 `X-Forwarded-Proto`、`X-Forwarded-For`、`Host`（容器内 nginx 已向上游转发这些头）；
- 将 `POLARIS_ISSUER` 设为对外 HTTPS 地址；
- 将 `POLARIS_COOKIE_SECURE=true`、`POLARIS_CSRF_ALLOWED_ORIGINS` 设为对外来源。

多租户：为每个租户配置独立域名并写入 `institutions.domain`；Polaris 按 `Host` 解析租户。

---

## 7. 健康检查与可观测

- 存活：`GET /healthz`；就绪：`GET /readyz`（含依赖状态）。镜像 `HEALTHCHECK` 走 `/healthz`。
- 日志：结构化（zap），关键链路带 `trace_id`（OpenTelemetry）。
- 审计：关键操作写入 `records` 表，可在控制台「审计」查看。

---

## 8. CI/CD 与发布

- `.github/workflows/ci.yml`：`go vet` + `go test ./...` + 前端 `pnpm test`（typecheck + vitest + build）。
- `.github/workflows/release.yml`：打 tag（`v*`）→ buildx 多架构（amd64/arm64）构建 → 推送 Docker Hub。

```bash
git tag v0.1.0 && git push origin v0.1.0
```

发布前需在仓库配置 Secrets `DOCKERHUB_USERNAME`、`DOCKERHUB_TOKEN`；镜像名可用仓库变量 `DOCKERHUB_IMAGE` 覆盖（默认 `<用户名>/polaris`）。

---

## 9. 升级与密钥轮换

1. 拉取新镜像并滚动替换（下方为有状态说明）。
2. **OIDC 签名密钥轮换（JWKS 平滑轮换）**：
   - 在控制台新增一张 JWT 证书；
   - 把新证书名加入 `POLARIS_OIDC_ADDITIONAL_KEY_CERT_NAMES` 并重启 —— 新公钥进入 JWKS，消费方可预取；
   - 把 `auth.oidc.cert_name` 切换为新证书并重启 —— 新令牌用新密钥签发；
   - 待旧令牌全部过期后，从附加列表与证书库移除旧证书。

---

## 10. 已知限制

- 供给 / 同步的进程内 scheduler 在**多实例**下会重复触发（后续拟引入 PG advisory lock）。
- 一体镜像的 nginx 仅监听 http；TLS 由前置代理负责。

## 提供商密钥回填（升级到第四阶段后执行一次）

第四阶段起，提供商（`providers` 表）的密钥改为 AES-GCM 加密存储。迁移 027 只改结构，
**存量明文密钥需要回填**；读取侧对"解密失败"按明文兼容，因此该命令可随时执行、可重复执行（已加密的行会跳过）：

```bash
POLARIS_CREDENTIAL_ENC_KEY=<与 app 相同> ./bin/polaris -rotate-provider-secrets
# 输出：提供商密钥回填完成：已加密 N 条，跳过 M 条
```

- 未配置 `POLARIS_CREDENTIAL_ENC_KEY` 时命令会直接报错退出（不会写入任何数据）；
- 回填前请确认 `configs/app.yaml` 的 `auth.oauth.providers` 已不再作为运行时权威——它只是**启动种子**
  （仅当目标组织尚无同名提供商时写入一次）。
