# ─────────────────────────────────────────────────────────────
# Polaris 一体镜像：后端（Go）+ 前端（React SPA）+ nginx（静态托管 + 反代）
#
# 说明：后端 API 与前端 SPA 原本分离（开发态用 Vite 代理 /api）。
# 生产镜像把两者合并：nginx 托管 SPA 并把 /api、/oauth、/.well-known、/static
# 反代到本机 :8100 的 Go 服务，从而单容器即可运行完整平台。
# ─────────────────────────────────────────────────────────────

# ── 1. 前端构建 ──────────────────────────────────────────────
# 前端产物与目标架构无关：固定在构建机上执行一次，再复制进 amd64/arm64 运行镜像，
# 避免 QEMU 下每个架构各跑一遍 pnpm build。
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@9 --activate
COPY frontend/web/package.json frontend/web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/web/ ./
RUN pnpm build

# ── 2. 后端构建 ──────────────────────────────────────────────
# 同样固定在构建机上交叉编译，避免 arm64 目标在 QEMU 里跑 Go 编译器。
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend
WORKDIR /src
# go.mod 声明 go 1.26，允许工具链自动下载；静态编译便于放入 alpine 运行镜像
ENV GOTOOLCHAIN=auto CGO_ENABLED=0
ARG TARGETOS=linux
ARG TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
# 仅复制后端所需目录（前端已在上一阶段单独构建）
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
COPY configs/ ./configs/
COPY migrations/ ./migrations/
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o /out/polaris ./cmd/polaris

# ── 3. 运行镜像 ──────────────────────────────────────────────
FROM nginx:1.27-alpine AS runtime

# 运行期依赖：bash（entrypoint）、tini（信号转发）、ca-certificates（出站 TLS）、tzdata
RUN apk add --no-cache bash tini ca-certificates tzdata && \
    rm -f /etc/nginx/conf.d/default.conf

WORKDIR /app

# 后端二进制、配置占位符与迁移脚本
COPY --from=backend /out/polaris /app/polaris
COPY configs/ /app/configs/
COPY migrations/ /app/migrations/

# 前端静态产物
COPY --from=web /src/dist /usr/share/nginx/html

# nginx 配置与入口脚本
COPY docker/nginx.conf /etc/nginx/conf.d/polaris.conf
COPY docker/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && mkdir -p /app/data

# 运行期配置：容器内以环境变量为准（POLARIS_* 覆盖 configs/app.yaml）
ENV POLARIS_CONFIG=/app/configs/app.yaml \
    POLARIS_MIGRATIONS_DIR=/app/migrations \
    POLARIS_PORT=8100 \
    POLARIS_DB_HOST=postgres \
    POLARIS_REDIS_HOST=redis

# 对外服务端口由容器内 nginx 提供；Go 后端只监听容器内 :8100。
EXPOSE 80

# 健康检查走 nginx 的 /readyz：同时验证静态站点、反向代理与依赖状态。
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1/readyz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/app/entrypoint.sh"]
