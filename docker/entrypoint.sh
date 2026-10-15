#!/usr/bin/env bash
# Polaris 容器入口：并行启动后端（Go）与 nginx，任一退出即整体优雅退出。
# 由 tini 作为 PID 1 负责信号转发与僵尸回收（见 Dockerfile ENTRYPOINT）。
set -uo pipefail

echo "[polaris] starting backend and nginx..."

/app/polaris &
BACKEND_PID=$!

nginx -g 'daemon off;' &
NGINX_PID=$!

shutdown() {
    echo "[polaris] shutting down..."
    kill -TERM "$BACKEND_PID" "$NGINX_PID" 2>/dev/null || true
    # 等待后端完成优雅关闭（containers 默认 10s，这里最多等 25s）
    for _ in $(seq 1 25); do
        kill -0 "$BACKEND_PID" 2>/dev/null || break
        sleep 1
    done
    kill -KILL "$BACKEND_PID" "$NGINX_PID" 2>/dev/null || true
    exit 0
}
trap shutdown TERM INT

# 等待任一子进程结束（bash 5.1+ 支持带 PID 的 wait -n）
wait -n "$BACKEND_PID" "$NGINX_PID"
STATUS=$?
echo "[polaris] a process exited (status=$STATUS), stopping container"
shutdown
