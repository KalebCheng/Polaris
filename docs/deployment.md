# 容器部署手册

> Polaris 以**单容器一体镜像**交付：Go 后端 + 前端 SPA + nginx。
> 本文覆盖镜像获取、Compose 体验、生产部署、数据库迁移、反向代理、升级回滚与 GitHub CI/CD。

---

## 1. 交付形态

镜像内部结构：

```
浏览器
  │
  ▼
nginx :80（容器对外唯一端口）
  ├── /                    前端 SPA（React 构建产物）
  ├── /api/*               反向代理 → Go 后端
  ├── /oauth/*             反向代理 → Go 后端
  ├── /.well-known/*       反向代理 → Go 后端
  └── /static/*            反向代理 → Go 后端
                             Go 后端监听容器内 :8100
```

- 对外只需要暴露 **80**（生产通常在它前面再做 TLS 终结）。
- 不要对外发布 `8100`：那是容器内部的 Go 后端端口，不包含前端静态资源与 nginx 路由。
- 外部依赖：**PostgreSQL 15+**、**Redis 7+**。
- 支持架构：`linux/amd64`、`linux/arm64`。

---

## 2. 镜像与标签

镜像仓库由 GitHub Actions 推送到 Docker Hub。默认镜像名为：

```text
<DOCKERHUB_USERNAME>/polaris
```

也可以在 GitHub 仓库变量 `DOCKERHUB_IMAGE` 中显式指定完整仓库名（不含 tag），例如
`kalebcheng/polaris`。下文示例统一使用 `kalebcheng`，**请替换为你实际的 Docker Hub 用户名或
`DOCKERHUB_IMAGE` 的值**。本项目文档示例使用：

```bash
docker pull kalebcheng/polaris:latest
```

> 若 Docker Hub 仓库为 Private，部署机拉取前需先执行 `docker login`。

标签规则：

| 触发方式 | 生成的标签 | 说明 |
|---|---|---|
| push 到 `main` | `main`、`sha-<短提交>` | 多架构推送，适合验证最新代码 |
| Pull Request | `pr-<编号>`、`sha-<短提交>` | **只构建，不推送** |
| push tag `1.2.3` | `1.2.3`、`1.2`、`latest`、`sha-<短提交>` | 正式发布，多架构推送；预发布 tag（含 `-`）不会更新 `latest` |
| 手动 Release（可选 tag） | `sha-<短提交>` + 手动标签 | 用于补发或预发布 |

生产环境建议固定完整版本（如 `1.2.3`），不要依赖 `latest`。

---

## 3. 必读配置

完整环境变量清单见 `README.md`。以下是最小生产集合：

| 变量 | 必填 | 说明 |
|---|---|---|
| `POLARIS_ENV` | 是 | 生产必须设为 `production` |
| `POLARIS_DB_HOST` | 是 | PostgreSQL 地址 |
| `POLARIS_DB_PORT` | 否 | 默认 `5432` |
| `POLARIS_DB_USER` | 是 | PostgreSQL 用户 |
| `POLARIS_DB_PASSWORD` | 是 | PostgreSQL 密码；允许显式设为空以适配特殊认证 |
| `POLARIS_DB_NAME` | 是 | 数据库名 |
| `POLARIS_DB_SSLMODE` | 否 | 默认 `disable`；托管数据库通常用 `require` |
| `POLARIS_DB_AUTO_MIGRATE` | 否 | 是否启动时自动迁移；生产建议 `false`，用一次性 Job 迁移 |
| `POLARIS_DB_SEED` | 首次安装建议 | 是否写入内置组织/管理员/证书种子；首次安装必须为 `true`，创建完成后建议改回 `false` |
| `POLARIS_REDIS_HOST` | 是 | Redis 地址 |
| `POLARIS_REDIS_PORT` | 否 | 默认 `6379` |
| `POLARIS_REDIS_DB` | 否 | 默认 `0` |
| `POLARIS_REDIS_PASSWORD` | 否 | Redis 密码；无认证 Redis 可显式设为空 |
| `POLARIS_JWT_SECRET` | 是 | 会话 JWT 密钥，建议 `openssl rand -base64 48` |
| `POLARIS_CREDENTIAL_ENC_KEY` | 生产必填 | AES-GCM 密钥，**base64 编码的 32 字节**；建议 `openssl rand -base64 32` |
| `POLARIS_ISSUER` | 是 | 对外访问地址，如 `https://sso.example.com`，OIDC issuer 使用 |
| `POLARIS_COOKIE_SECURE` | 是 | HTTPS 环境必须为 `true` |
| `POLARIS_CSRF_ALLOWED_ORIGINS` | 是 | 允许携带 Cookie 执行写操作的来源，逗号分隔 |
| `POLARIS_ADMIN_PASSWORD` | 建议 | 种子管理员密码；仅在 `POLARIS_DB_SEED=true` 时生效 |
| `POLARIS_MIGRATIONS_DIR` | 否 | 镜像内默认 `/app/migrations` |
| `POLARIS_CONFIG` | 否 | 镜像内默认 `/app/configs/app.yaml` |

> **`POLARIS_CREDENTIAL_ENC_KEY` 必须备份。** 提供商密钥、同步器密钥、存储密钥、TOTP 密钥都依赖它解密；
> 丢失后这些数据无法恢复。生产环境未配置该变量时应用会拒绝启动。

> 外部身份提供方（钉钉/微信/飞书/OIDC 等）的权威配置在控制台「提供商」页面；`POLARIS_OAUTH_*`
> 仅用于首次启动的种子写入，修改后请重启或直接以控制台为准。

---

## 4. 快速体验：Docker Compose

仓库根目录的 `docker-compose.yml` 会同时拉起 PostgreSQL、Redis 和应用镜像，适合本地体验或单机验证。

```bash
cp .env.example .env
# 编辑 .env：至少替换 POLARIS_IMAGE、数据库/Redis 密码、JWT 密钥、加密密钥、管理员密码
docker compose pull
docker compose up -d
docker compose ps
curl -fsS http://localhost:8100/readyz
```

访问 `http://localhost:8100`：

- 组织：`built-in`
- 管理员：`admin`
- 密码：`.env` 中的 `POLARIS_ADMIN_PASSWORD`

首次启动会用 `POLARIS_ADMIN_PASSWORD` 创建管理员；登录后请尽快修改密码，并把 `POLARIS_DB_SEED` 改为 `false`。

> Compose 默认把 PostgreSQL `5432`、Redis `6379` 发布到宿主机，仅方便本地排查。
> 面向公网的单机部署应删除这两个端口映射，或至少绑定到 `127.0.0.1`。

常用命令：

```bash
docker compose logs -f app
docker compose down            # 停止并保留 PostgreSQL 数据卷
docker compose down -v         # 停止并删除数据卷（会清空数据）
```

---

## 5. 生产部署：外部依赖 + docker run

推荐使用外部托管 PostgreSQL/Redis，只运行 Polaris 应用容器。
先把环境变量写入受保护的文件（权限 `600`），避免密钥出现在 shell history：

```bash
sudo install -m 600 /dev/null /etc/polaris/polaris.env
sudo tee /etc/polaris/polaris.env >/dev/null <<'EOF'
POLARIS_ENV=production
POLARIS_DB_HOST=pg.internal
POLARIS_DB_PORT=5432
POLARIS_DB_USER=polaris
POLARIS_DB_PASSWORD=replace-me
POLARIS_DB_NAME=polaris
POLARIS_DB_SSLMODE=require
POLARIS_DB_AUTO_MIGRATE=false
# 首次安装设为 true 以创建内置组织/管理员；成功后改为 false
POLARIS_DB_SEED=true
POLARIS_ADMIN_PASSWORD=replace-with-strong-admin-password

POLARIS_REDIS_HOST=redis.internal
POLARIS_REDIS_PORT=6379
POLARIS_REDIS_PASSWORD=replace-me

POLARIS_JWT_SECRET=replace-me
POLARIS_CREDENTIAL_ENC_KEY=replace-me-with-base64-32-bytes
POLARIS_ISSUER=https://sso.example.com
POLARIS_COOKIE_SECURE=true
POLARIS_CSRF_ALLOWED_ORIGINS=https://sso.example.com
EOF
sudo chmod 600 /etc/polaris/polaris.env
```

> 首次安装：`POLARIS_DB_SEED=true` + 强 `POLARIS_ADMIN_PASSWORD`，启动后确认管理员可登录，
> 再把 `POLARIS_DB_SEED` 改为 `false` 并重启。种子逻辑幂等：内置组织已存在时会直接跳过。

启动容器（只绑定本机回环地址，由前置反向代理访问）：

```bash
docker run -d --name polaris \
  --restart unless-stopped \
  --env-file /etc/polaris/polaris.env \
  -p 127.0.0.1:8100:80 \
  --stop-timeout 30 \
  kalebcheng/polaris:1.2.3
```

验证：

```bash
docker ps --filter name=polaris
docker logs --tail 100 polaris
curl -fsS http://127.0.0.1:8100/healthz
curl -fsS http://127.0.0.1:8100/readyz
```

> 端口映射必须是 `宿主机端口:80`。映射到 `:8100` 会把容器内的 Go 后端直接暴露出去，
> 前端页面和 nginx 路由都不可用。

---

## 6. 数据库迁移

迁移由内置 `pkg/migrator` 驱动：每个迁移独立事务、advisory lock 防并发、记录写入 `schema_migrations`。

两种模式二选一：

1. **启动时自动迁移**：`POLARIS_DB_AUTO_MIGRATE=true`，适合单实例首次部署或开发环境。
2. **生产推荐：独立一次性 Job**：保持应用 `POLARIS_DB_AUTO_MIGRATE=false`，发布前单独执行迁移。

应用全部未执行迁移：

```bash
docker run --rm \
  --env-file /etc/polaris/polaris.env \
  --entrypoint /app/polaris \
  kalebcheng/polaris:1.2.3 \
  -migrate
```

回滚最近 1 步（谨慎操作）：

```bash
docker run --rm \
  --env-file /etc/polaris/polaris.env \
  --entrypoint /app/polaris \
  kalebcheng/polaris:1.2.3 \
  -migrate-down=1
```

本次基线变更需要已有库手动订正 DDL 与数据；`001_init` 不会在已执行过的库上自动重放。
升级前应停止旧实例，按新基线调整 `tokens` / `applications` / `institutions` 的列，并确保
`certs.private_key` 与 `institutions.default_password_enc` 保存的是 AES-GCM 密文。
本次的单向升级脚本见 `docs/upgrades/sensitive-storage-hardening.up.sql`。

约束：

- 迁移文件不可变，已发布迁移禁止修改，只能新增。
- 多实例滚动发布时，先完成迁移再启动新版本；迁移器自身有 advisory lock，但仍建议串行发布。
- 执行迁移前先备份数据库；回滚镜像版本**不会自动回滚数据库结构**。

---

## 7. 反向代理与 TLS

容器内 nginx 只监听 HTTP `:80`，TLS 由前置代理（云 LB / Nginx / Traefik）负责。

Nginx 示例：

```nginx
server {
    listen 443 ssl http2;
    server_name sso.example.com;

    ssl_certificate     /etc/letsencrypt/live/sso.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/sso.example.com/privkey.pem;

    client_max_body_size 8m;

    location / {
        proxy_pass http://127.0.0.1:8100;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection        "";
    }
}
```

配套环境变量：

```bash
POLARIS_ISSUER=https://sso.example.com
POLARIS_COOKIE_SECURE=true
POLARIS_CSRF_ALLOWED_ORIGINS=https://sso.example.com
```

要点：

- `POLARIS_ISSUER` 必须是浏览器实际访问的**外部 HTTPS 地址**，否则 OIDC discovery、ID Token issuer、
  登录回跳可能不一致。
- `POLARIS_CSRF_ALLOWED_ORIGINS` 必须包含完整 origin（协议 + 域名 + 端口），多个用逗号分隔。
- 容器内 nginx 会保留前置代理传来的 `X-Forwarded-Proto`；不要把 TLS 在容器内重复终结。
- 多租户按 `Host` 解析组织；为每个租户配置独立域名并写入 `institutions.domain`。

---

## 8. 健康检查、日志与数据

- 存活探针：`GET /healthz`
- 就绪探针：`GET /readyz`（包含 PostgreSQL / Redis 依赖状态）
- 镜像内置 `HEALTHCHECK`，走 nginx 的 `/readyz`。
- 日志输出到 stdout，结构化 JSON，关键链路带 `trace_id`；由宿主机/采集器负责轮转与收集。
- PostgreSQL 是唯一业务主存储，必须持久化并备份；Redis 保存会话、OAuth state 与限流计数，
  可按可重建数据处理，但清空会导致所有用户掉线。

---

## 9. 升级与回滚

升级步骤：

```bash
# 1. 备份数据库
# 2. 拉取固定版本镜像
docker pull kalebcheng/polaris:1.3.0

# 3. 执行迁移（应用 POLARIS_DB_AUTO_MIGRATE=false 时）
docker run --rm --env-file /etc/polaris/polaris.env \
  --entrypoint /app/polaris kalebcheng/polaris:1.3.0 -migrate

# 4. 替换容器
docker rm -f polaris
docker run -d --name polaris \
  --restart unless-stopped \
  --env-file /etc/polaris/polaris.env \
  -p 127.0.0.1:8100:80 \
  --stop-timeout 30 \
  kalebcheng/polaris:1.3.0

# 5. 验证
curl -fsS http://127.0.0.1:8100/readyz
```

回滚镜像前确认新版本迁移是否向后兼容；如不兼容，需要先用 `-migrate-down=N` 回滚数据库结构。

### OIDC 签名密钥平滑轮换

1. 在控制台新增一张 JWT 证书；
2. 把新证书名加入 `POLARIS_OIDC_ADDITIONAL_KEY_CERT_NAMES` 并重启：新公钥进入 JWKS，消费方可预取；
3. 把 `auth.oidc.cert_name` 切换为新证书并重启：新令牌改用新密钥签发；
4. 待旧令牌全部过期后，从附加列表与证书库移除旧证书。

---

## 10. GitHub CI/CD 与 Docker Hub

### 10.1 工作流

| 工作流 | 触发 | 内容 |
|---|---|---|
| `.github/workflows/ci.yml` | 仅被 `release.yml` 调用，或手动触发 | 后端 `go vet` + 单元测试 + 编译；前端 typecheck + vitest + build。**不再响应 `main` push 或 PR** |
| `.github/workflows/release.yml` | push 任意 tag、手动 | 先调用 CI 测试；通过后构建 `linux/amd64,linux/arm64` 镜像并推送版本标签与 `latest`；打 tag 时自动创建 GitHub Release（含 `-` 的 tag 标记为 prerelease） |

### 10.2 仓库配置

在 GitHub 仓库 **Settings → Secrets and variables → Actions** 中配置：

| 类型 | 名称 | 值 |
|---|---|---|
| Secret | `DOCKERHUB_USERNAME` | Docker Hub 用户名 |
| Secret | `DOCKERHUB_TOKEN` | Docker Hub Access Token（至少 Read & Write） |
| Variable（可选） | `DOCKERHUB_IMAGE` | 完整镜像仓库名（不含 tag），如 `kalebcheng/polaris`；不配置时默认 `<用户名>/polaris` |

Docker Hub 侧：

1. 在 Docker Hub 创建仓库（例如 `polaris`，Public 或 Private 均可）；
2. Account Settings → Security → New Access Token，选择 **Read & Write**；
3. 把用户名与 Token 填入上面的 GitHub Secrets。

> 不需要也不应该把 Docker Hub 密码写进仓库文件。PR 来自 fork 时拿不到 Secrets，
> 因此 PR 只做镜像构建验证，不登录、不推送。

### 10.3 发布新版本

```bash
git tag 1.2.3
git push origin 1.2.3
```

发布完成后 Docker Hub 会出现镜像标签，同时 GitHub 的 **Releases** 页面会出现对应发布记录：

```text
kalebcheng/polaris:1.2.3
kalebcheng/polaris:1.2
kalebcheng/polaris:latest
kalebcheng/polaris:sha-<短提交>
```

也可以走手动发布：Actions → **Release** → Run workflow，可选择是否推送、填写附加标签。

### 10.4 本地构建与推送

```bash
docker build -t kalebcheng/polaris:dev .
docker run --rm -p 8100:80 \
  -e POLARIS_ENV=production \
  -e POLARIS_DB_HOST=host.docker.internal \
  -e POLARIS_DB_PASSWORD="$POLARIS_DB_PASSWORD" \
  -e POLARIS_REDIS_HOST=host.docker.internal \
  -e POLARIS_REDIS_PASSWORD=redis \
  -e POLARIS_JWT_SECRET=dev-secret \
  -e POLARIS_CREDENTIAL_ENC_KEY="$(openssl rand -base64 32)" \
  kalebcheng/polaris:dev
```

---

## 11. 故障排查

| 现象 | 排查方向 |
|---|---|
| 打开页面 `502 Bad Gateway` | `/readyz` 是否通过；PostgreSQL/Redis 连通性；`docker logs polaris` |
| 容器启动后立即退出 | 环境变量缺失、数据库连接失败、`POLARIS_CREDENTIAL_ENC_KEY` 非法或迁移失败 |
| 登录后立刻 401 / 写操作被 CSRF 拒绝 | `POLARIS_ISSUER`、`POLARIS_CSRF_ALLOWED_ORIGINS`、`POLARIS_COOKIE_SECURE` 是否与外部地址一致 |
| Redis 报 `NOAUTH` / `ERR AUTH` | `POLARIS_REDIS_PASSWORD` 与 Redis 实际密码不一致；无密码时显式设为空 |
| 迁移失败 | 数据库用户权限、`schema_migrations` 状态、迁移目录是否完整；先备份再处理 |
| OIDC 回调地址不一致 | IdP 后台登记的回调地址必须与 Provider 配置逐字符一致；不要把 `from` 等参数拼进 `redirect_uri` |
| 前端刷新 404 | 确认请求进入了容器 nginx，而不是直接访问 `:8100` 后端端口 |

---

## 12. 已知限制

- 供给 / 同步的进程内 scheduler 在**多实例**下仍可能重复触发（后续拟引入 PostgreSQL advisory lock）；
- 一体镜像内 nginx 只监听 HTTP，TLS 必须由前置代理负责；
- 单容器内 nginx 与 Go 后端同生命周期，容器退出策略由 entrypoint 统一处理。
