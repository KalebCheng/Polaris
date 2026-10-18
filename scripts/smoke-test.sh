#!/usr/bin/env bash
# Polaris 端到端冒烟测试
# 用法：BASE=http://localhost:8100 ./scripts/smoke-test.sh
set -euo pipefail

BASE="${BASE:-http://localhost:8100}"
ORG="${ORG:-built-in}"
APP="${APP:-app-built-in}"
ADMIN="${ADMIN:-admin}"
PASSWORD="${PASSWORD:-admin123}"
CLIENT_ID="${CLIENT_ID:-polaris-built-in-client}"
CLIENT_SECRET="${CLIENT_SECRET:-polaris-built-in-secret}"
JAR="$(mktemp "${TMPDIR:-/tmp}/polaris-smoke.XXXXXX")"
RUN_ID="$(date +%s)-$$"
TEST_USER="smoke-user-$RUN_ID"
TEST_ROLE="smoke-role-$RUN_ID"
TEST_DEPT="smoke-dept-$RUN_ID"
TEST_DEPT_CHILD="smoke-dept-child-$RUN_ID"
TEST_APP="smoke-app-$RUN_ID"
TEST_SIGNUP_USER="smoke-signup-$RUN_ID"
TEST_USER5="smoke-user5-$RUN_ID"
TEST_ACCESS_USER="smoke-access-$RUN_ID"
TEST_ACCESS_DEPT="smoke-access-dept-$RUN_ID"
TEST_ACCESS_DEPT_CHILD="smoke-access-dept-child-$RUN_ID"
TEST_DEPT5="smoke-dept5-$RUN_ID"
TEST_LEVEL5="smoke-level-$(date +%s)"
# 组织内手机号唯一（迁移 039）：冒烟号每次运行都必须不同
SMOKE_PHONE5="139$(printf '%08d' $((RUN_ID % 100000000)))"
TEST_GROUP5="smoke-group5-$RUN_ID"
TEST_STORAGE="smoke-storage-$RUN_ID"
TEST_STORAGE2="smoke-storage2-$RUN_ID"
TEST_PLAN_SYNCER="smoke-plan-$RUN_ID"
TEST_CAPTCHA="smoke-captcha-$RUN_ID"
TEST_EMAIL="smoke-email-$RUN_ID"
TEST_SMS="smoke-sms-$RUN_ID"
TEST_CONNECTOR="smoke-conn-$RUN_ID"
TEST_STRATEGY="smoke-strategy-$RUN_ID"
TEST_PROVIDERS=""
# 资源路径参数统一用数字主键 id（创建后从响应里取）
USER_ID=""; SIGNUP_USER_ID=""; ROLE_ID=""; DEPT_ID=""; DEPT_CHILD_ID=""
USER5_ID=""; DEPT5_ID=""; GROUP5_ID=""; ADMIN_ID=""
BUILTIN_APP_ID=""; TEST_APP_ID=""; STORAGE_ID=""; CAPTCHA_ID=""; EMAIL_ID=""
CONNECTOR_ID=""; STRATEGY_ID=""; ORG_ID=""
REDIRECT_URI="${REDIRECT_URI:-http://localhost:5173/oauth/callback}"

# 退出时清理测试资源（含失败场景）：残留的 Captcha 提供商会让后续登录要求验证码
cleanup() {
  if [ -n "${JAR:-}" ] && [ -f "$JAR" ]; then
    for PID in ${TEST_PROVIDERS:-}; do
      curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/providers/$PID" >/dev/null 2>&1 || true
    done
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/users/${USER_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/users/${SIGNUP_USER_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/job-titles/${GROUP5_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/departments/${DEPT5_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/users/${USER5_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/users/${ACCESS_USER_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/departments/${ACCESS_DEPT_CHILD_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/departments/${ACCESS_DEPT_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/connectors/${CONNECTOR_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/account-strategies/${STRATEGY_ID:-0}" >/dev/null 2>&1 || true
    curl -sS -b "$JAR" -X DELETE "$BASE/api/v1/applications/${TEST_APP_ID:-0}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

green() { printf '\033[32m%s\033[0m\n' "$1"; }
red()   { printf '\033[31m%s\033[0m\n' "$1"; }
step()  { printf '\n\033[36m── %s\033[0m\n' "$1"; }

expect_code() { # expect_code <expected> <actual> <label>
  if [ "$1" = "$2" ]; then
    green "PASS  $3 ($2)"
  else
    red "FAIL  $3 (期望 $1，实际 $2)"
    [ -f /tmp/polaris-smoke-body ] && red "响应体: $(cat /tmp/polaris-smoke-body)"
    exit 1
  fi
}

code() { : > /tmp/polaris-smoke-body; curl -sS -o /tmp/polaris-smoke-body -w '%{http_code}' "$@"; }

# expect_ok <label> <python 代码>：代码需 print OK 表示通过（其余视为失败原因）
expect_ok() {
  local out
  out="$(python3 -c "$2" 2>&1)"
  if [ "$out" = "OK" ]; then
    green "PASS  $1"
  else
    red "FAIL  $1（${out:-}）"
    exit 1
  fi
}

# body_json <表达式>：在响应体 data 上求值（用于断言）
body_json() {
  python3 -c "import json,sys
d = json.load(open('/tmp/polaris-smoke-body'))
print(d.get('data', {}).$1)" 2>&1
}

# login_body <institution> <username> <password> [application]
login_body() {
  if [ -n "${4:-}" ]; then
    printf '{"institution":"%s","application":"%s","username":"%s","password":"%s"}' "$1" "$4" "$2" "$3"
  else
    printf '{"institution":"%s","username":"%s","password":"%s"}' "$1" "$2" "$3"
  fi
}

# create_user_body <institution> <name> <display> <email> <password>
create_user_body() {
  printf '{"institution_id":%s,"name":"%s","display_name":"%s","email":"%s","password":"%s"}' "$1" "$2" "$3" "$4" "$5"
}

# json_field <key>：从最近一次响应体的 data 中取字段
json_field() {
  python3 -c 'import json,sys
try:
    d = json.load(open("/tmp/polaris-smoke-body"))
    print(d.get("data", {}).get(sys.argv[1], ""))
except Exception:
    print("")' "$1"
}

# json_items_field <field>：从最近一次响应体的 data.items[0] 取字段（列表类端点用；空列表返回空串）
json_items_field() {
  python3 -c 'import json,sys
try:
    d = json.load(open("/tmp/polaris-smoke-body"))
    items = d.get("data", {}).get("items") or []
    print(items[0].get(sys.argv[1], "") if items else "")
except Exception:
    print("")' "$1"
}

# json_job_id_with_runs：从最近一次 /asyncd/jobs 响应里挑第一个"有执行记录"的任务定义主键
json_job_id_with_runs() {
  python3 -c 'import json
try:
    d = json.load(open("/tmp/polaris-smoke-body"))
    items = d.get("data", {}).get("items") or []
    for item in items:
        if (item.get("stats") or {}).get("total", 0) > 0:
            print(item.get("id", ""))
            break
    else:
        print("")
except Exception:
    print("")'
}

# json_owner_job_id <syncer_id>：从最近一次 /asyncd/jobs 响应取"归属该连接"的任务定义主键
json_owner_job_id() {
  python3 -c 'import json,sys
try:
    d = json.load(open("/tmp/polaris-smoke-body"))
    items = d.get("data", {}).get("items") or []
    for item in items:
        if item.get("owner_syncer_id") == int(sys.argv[1]):
            print(item.get("id", ""))
            break
    else:
        print("")
except Exception:
    print("")' "$1"
}

# json_top <key>：从最近一次响应体顶层取字段（OIDC 端点返回标准 JSON，无统一包装）
json_top() {
  python3 -c 'import json,sys
try:
    d = json.load(open("/tmp/polaris-smoke-body"))
    print(d.get(sys.argv[1], ""))
except Exception:
    print("")' "$1"
}

# list_id <json-key> <value>：从最近一次"列表"响应中按字段值取该资源的 id
list_id() {
  python3 -c 'import json,sys
try:
    items = json.load(open("/tmp/polaris-smoke-body")).get("data", {}).get("items", [])
    for item in items:
        if item.get(sys.argv[1]) == sys.argv[2]:
            print(item.get("id", ""))
            break
except Exception:
    pass' "$1" "$2"
}

# json_field <key>：从最近一次响应体（/tmp/polaris-smoke-body）的 data 中取字段
json_field() {
  python3 -c 'import json,sys
try:
    d = json.load(open("/tmp/polaris-smoke-body"))
    print(d.get("data", {}).get(sys.argv[1], ""))
except Exception:
    print("")' "$1"
}

step "1. 健康检查"
expect_code 200 "$(code "$BASE/healthz")" "GET /healthz"
expect_code 200 "$(code "$BASE/readyz")" "GET /readyz"

step "2. OIDC 元数据"
expect_code 200 "$(code "$BASE/.well-known/openid-configuration")" "GET /.well-known/openid-configuration"
expect_code 200 "$(code "$BASE/.well-known/jwks")" "GET /.well-known/jwks"
expect_code 200 "$(code "$BASE/api/v1/auth/institutions")" "GET /auth/institutions（登录页公开端点）"
expect_code 200 "$(code "$BASE/api/v1/auth/providers")" "GET /auth/providers（登录页公开端点）"

step "3. 错误密码被拒绝"
expect_code 401 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(login_body "$ORG" "$ADMIN" 'wrong-password')")" "错误密码 → 401"

step "4. 未认证访问受保护资源"
expect_code 401 "$(code "$BASE/api/v1/users")" "未登录访问 /api/v1/users → 401"

step "5. 密码登录（不指定组织，由后端自动解析）"
expect_code 200 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"username":"%s","password":"%s"}' "$ADMIN" "$PASSWORD")")" \
  "省略 institution 登录 → 200（自动解析到唯一组织）"

step "6. 密码登录（显式指定组织）"
expect_code 200 "$(code -c "$JAR" -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(login_body "$ORG" "$ADMIN" "$PASSWORD" "$APP")")" "登录成功 → 200"

step "7. 会话状态"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/auth/status")" "GET /auth/status → 200"

step "8. 当前用户（含权限）"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/me")" "GET /me → 200"

step "9. 管理 API（RBAC 管理员直通）"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/institutions")" "GET /institutions（解析组织 id）→ 200"
ORG_ID="$(list_id name "$ORG")"
if [ -z "$ORG_ID" ]; then red "FAIL  未取到组织 id"; exit 1; fi
green "PASS  当前组织 id = $ORG_ID"

step "9b. 公开面组织引用：institution_id（主键）为权威，institution（标识）兼容"
expect_code 200 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"username":"%s","password":"%s","application":"%s"}' "$ORG_ID" "$ADMIN" "$PASSWORD" "$APP")")" \
  "按组织主键 institution_id 登录 → 200"
expect_code 404 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw '{"institution_id":999999,"username":"admin","password":"wrong"}' )" \
  "错误组织主键 → 404（不得静默回落到其它组织）"

expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users")" "GET /users → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/institutions")" "GET /institutions → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/departments")" "GET /departments → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications")" "GET /applications → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/roles")" "GET /roles → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/permissions")" "GET /permissions → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/job-titles")" "GET /job-titles → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/certs")" "GET /certs → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/syncers/types")" "GET /syncers/types → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/records")" "GET /records → 200"
# 身份连接目录（类型与字段 schema 的单一来源）
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/connections/catalog")" "GET /connections/catalog → 200"
# 平台接入信息（开发信息：回调地址 / 门户地址，供回填到 IdP 后台）
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/system/oauth")" "GET /system/oauth → 200"
# asyncd：四维度（任务定义 / 执行实例 / 执行记录 / 执行器实例）
# 任务目录：顺带取一个"有执行记录"的定义主键，下一条用它查执行实例（嵌套路径）
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/asyncd/jobs")" "GET /asyncd/jobs → 200"
ASYNCD_JOB_ID="$(json_job_id_with_runs)"
if [ -n "$ASYNCD_JOB_ID" ]; then
  expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/asyncd/jobs/$ASYNCD_JOB_ID/runs?size=1")" "GET /asyncd/jobs/:id/runs → 200"
  ASYNCD_RUN_ID="$(json_items_field id)"
  if [ -n "$ASYNCD_RUN_ID" ]; then
    expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/asyncd/jobs/$ASYNCD_JOB_ID/runs/$ASYNCD_RUN_ID/attempts")" "GET /asyncd/jobs/:id/runs/:runId/attempts → 200"
  else
    green "SKIP  asyncd 暂无执行实例，跳过 attempts 用例（路由仍由 router_test 覆盖）"
  fi
fi
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/asyncd/workers")" "GET /asyncd/workers → 200"

# 身份连接的「同步计划」就是 asyncd 的一条定时任务（Spec：261015-syncer-schedule-into-asyncd）
step "9a. 连接的同步计划 → asyncd 定时任务（Cron 一等能力）"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/syncers" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟计划连接","type":"ldap","sub_type":"openldap","is_enabled":true,"host":"127.0.0.1","port":389,"user":"cn=admin,dc=example,dc=com","base_dn":"dc=example,dc=com","sync_config":{"schedule":{"mode":"cron","cron":"0 4 * * *"}}}' "$ORG_ID" "$TEST_PLAN_SYNCER")")" \
  "创建身份连接（cron 计划）→ 201"
PLAN_SYNCER_ID="$(json_field id)"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/asyncd/jobs")" "GET /asyncd/jobs（取连接的计划任务）→ 200"
PLAN_JOB_ID="$(json_owner_job_id "$PLAN_SYNCER_ID")"
if [ -z "$PLAN_JOB_ID" ]; then red "FAIL  保存连接后未生成计划任务（配置即生效被破坏）"; exit 1; fi
green "PASS  保存连接即生成 asyncd 定时任务（job=${PLAN_JOB_ID}）"

# 计划任务必须：按 cron 规则落库、复用 syncer.run 处理器（到点即同步一跳）、来源可读
export PLAN_JOB_ID PLAN_SYNCER_NAME="$TEST_PLAN_SYNCER"
expect_ok "计划任务规则/处理器/来源正确" 'import json,os
d=json.load(open("/tmp/polaris-smoke-body"))
items=d.get("data",{}).get("items") or []
job=next((j for j in items if str(j.get("id"))==os.environ["PLAN_JOB_ID"]), None)
assert job, "plan job not found"
assert job["schedule"]["kind"]=="cron" and job["schedule"]["cron_expr"]=="0 4 * * *", job["schedule"]
assert job["task_type"]=="syncer.run", job
assert job.get("owner_name")==os.environ["PLAN_SYNCER_NAME"], job
print("OK")'

expect_code 400 "$(code -b "$JAR" -X PUT "$BASE/api/v1/asyncd/jobs/$PLAN_JOB_ID" -H 'Content-Type: application/json' \
  --data-raw '{"enabled":false}')" "归属连接的计划任务拒绝界面启停 → 400"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/syncers/$PLAN_SYNCER_ID")" "删除连接 → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/asyncd/jobs")" "GET /asyncd/jobs（确认无孤儿任务）→ 200"
ORPHAN_JOB="$(json_owner_job_id "$PLAN_SYNCER_ID")"
if [ -n "$ORPHAN_JOB" ]; then red "FAIL  连接已删除但计划任务仍在（孤儿任务）"; exit 1; fi
green "PASS  删除连接后计划任务随外键级联清除（无孤儿）"

step "10. 用户生命周期（创建 / 查询 / 重置密码 / 删除）"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/users" -H 'Content-Type: application/json' \
  --data-raw "$(create_user_body "$ORG_ID" "$TEST_USER" 冒烟用户 "smoke-$RUN_ID@polaris.local" 'Smoke@12345')")" "创建用户 → 201"
USER_ID="$(json_field id)"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER_ID")" "查询用户（按 id）→ 200"
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/users/$USER_ID/reset-password" -H 'Content-Type: application/json' \
  --data-raw '{"new_password":"Smoke@54321"}')" "重置密码 → 200"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/users/$USER_ID")" "删除用户 → 200"

step "10b. 用户详情与安全动作（第五阶段：详情 / 部分更新 / 重置 MFA / 解除锁定 / 授权 / 归属）"
# 整页表单：一次提交完整资料字段（后端若丢弃任一字段，下面的断言会红）
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/users" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"阶段五用户","first_name":"Smoke5","last_name":"User5","email":"smoke5-%s@polaris.local","phone":"%s","country_code":"86","location":"杭州","address":"文一西路 1 号","affiliation":"冒烟机构","title":"smoke-title","homepage":"https://example.com/smoke5","bio":"smoke bio","language":"zh","gender":"other","birthday":"2000-01-01","education":"BSc","password":"Smoke5@12345"}' "$ORG_ID" "$TEST_USER5" "$RUN_ID" "$SMOKE_PHONE5")")" "创建用户（完整资料字段）→ 201"
USER5_ID="$(json_field id)"
expect_ok "新建提交的全部资料字段都已落库（表单能填、后端不丢）" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
want = {'first_name': 'Smoke5', 'last_name': 'User5', 'country_code': '86', 'location': '杭州',
        'address': '文一西路 1 号', 'affiliation': '冒烟机构', 'homepage': 'https://example.com/smoke5',
        'bio': 'smoke bio', 'gender': 'other', 'birthday': '2000-01-01', 'education': 'BSc'}
bad = {k: d.get(k) for k, v in want.items() if d.get(k) != v}
print('OK' if not bad else '字段不一致: %s' % bad)"

expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID")" "GET 用户详情 → 200"
expect_ok "详情 created_at 是真实时间（零值时间戳回归）" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
print('OK' if d.get('created_at', '')[:4] >= '2000' else 'created_at=%r' % d.get('created_at'))"

# 部分更新：只提交 title，其余字段必须保持不变（D-1 全量覆盖回归）
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/users/$USER5_ID" -H 'Content-Type: application/json' \
  --data-raw '{"title":"smoke-title"}')" "PUT 仅提交 title → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID")" "PUT 后回读详情 → 200"
expect_ok "未提交字段未被清空（display_name / email 保留）" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
bad = [k for k in ('display_name', 'email', 'name') if not d.get(k)]
print('OK' if not bad and d.get('title') == 'smoke-title' else '被清空=%s title=%r' % (bad, d.get('title')))"

# 账号名是标识锚点：即使请求体里带上也不允许被改（服务层不接收该字段）
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/users/$USER5_ID" -H 'Content-Type: application/json' \
  --data-raw '{"name":"smoke5-renamed"}' )" "PUT 带 name 字段（应被忽略）→ 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID")" "账号名不可改回读 → 200"
expect_ok "账号名保持不变" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
print('OK' if d.get('name') == '$TEST_USER5' else 'name=%r' % d.get('name'))"

expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID/security")" "GET 安全与认证概览 → 200"
expect_ok "安全概览含 lockouts 数组" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
print('OK' if isinstance(d.get('lockouts'), list) and 'password' in d else 'shape=%s' % sorted(d.keys()))"
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/users/$USER5_ID/reset-mfa")" "重置 MFA → 200"
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/users/$USER5_ID/unlock")" "解除登录限制（无记录）→ 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID/authorizations")" "GET 授权全景 → 200"
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/users/$USER5_ID/roles" -H 'Content-Type: application/json' \
  --data-raw '{"roles":["user"]}')" "设置直接角色 → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID/authorizations")" "回读授权（含来源）→ 200"
expect_ok "直接角色生效且有有效权限" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
print('OK' if d.get('direct_roles') == ['user'] and d.get('effective_permissions') else 'roles=%r effective=%d' % (d.get('direct_roles'), len(d.get('effective_permissions') or [])))"

# 职务归属 + 组详情（成员 / 关联角色 / 汇总权限）
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/job-titles" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟职务"}' "$ORG_ID" "$TEST_GROUP5")")" "创建职务 → 201"
GROUP5_ID="$(json_field id)"
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/users/$USER5_ID/job-titles" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"job_titles":["%s"]}' "$TEST_GROUP5")")" "设置用户职务归属 → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/job-titles/$GROUP5_ID/detail")" "GET 职务详情 → 200"
expect_ok "组详情含成员明细与权限字段" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
names = [m.get('name') for m in d.get('members', [])]
print('OK' if names == ['$TEST_USER5'] and isinstance(d.get('permissions'), list) else 'members=%r' % names)"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/job-titles")" "GET 职务列表（含任职人数）→ 200"

# 职级（Spec 第六阶段 Q4：字典 + 用户单值属性；不参与授权）
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/job-levels" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","description":"冒烟职级","sort_order":100}' "$ORG_ID" "$TEST_LEVEL5")")" "创建职级 → 201"
LEVEL5_ID="$(json_field id)"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/job-levels")" "GET 职级列表 → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/job-levels/distribution")" "GET 职级分布 → 200"
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/users/$USER5_ID/job-level" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"job_level_id":%s}' "$LEVEL5_ID")")" "设置用户职级 → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID")" "读回用户（含职级）→ 200"
expect_ok "用户职级已落库并可读回" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
print('OK' if d.get('job_level_id') == $LEVEL5_ID else 'job_level_id=%r' % d.get('job_level_id'))"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/departments")" "GET 部门列表（取组织根）→ 200"
ROOT_DEPT_ID="$(python3 -c "import json;d=json.load(open('/tmp/polaris-smoke-body'))['data']['items'];print(next((i['id'] for i in d if i.get('is_root')), ''))")"
if [ -z "$ROOT_DEPT_ID" ]; then
  red "组织根部门缺失（每个组织必须恰好一行 is_root）"
  exit 1
fi
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/users/$USER5_ID/departments" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"department_ids":[%s],"primary_department_id":%s}' "$ROOT_DEPT_ID" "$ROOT_DEPT_ID")")" "设置用户部门归属（多值）→ 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID")" "读回用户（含部门归属）→ 200"
expect_ok "用户主部门已随多值归属同步" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
print('OK' if d.get('department_id') == $ROOT_DEPT_ID else 'department_id=%r' % d.get('department_id'))"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/job-levels/$LEVEL5_ID")" "删除冒烟职级 → 200"
expect_ok "组列表下发 member_count" "
import json
items = json.load(open('/tmp/polaris-smoke-body'))['data']['items']
print('OK' if items and all('member_count' in item for item in items) else 'items=%d' % len(items))"

# 部门成员整体替换
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/departments" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"name":"%s","display_name":"冒烟部门五"}' "$TEST_DEPT5")")" "创建部门（阶段五）→ 201"
DEPT5_ID="$(json_field id)"
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/departments/$DEPT5_ID/members" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"users":["%s"]}' "$TEST_USER5")")" "设置部门成员 → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/departments/$DEPT5_ID/members")" "GET 部门成员 → 200"
expect_ok "部门成员已挂载" "
import json
names = [u.get('name') for u in json.load(open('/tmp/polaris-smoke-body'))['data']['items']]
print('OK' if names == ['$TEST_USER5'] else 'members=%r' % names)"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID")" "回读用户详情（含部门路径）→ 200"
expect_ok "user.department_path 已补全" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
print('OK' if d.get('department_path') else 'department_path=%r department_id=%r' % (d.get('department_path'), d.get('department_id')))"
expect_code 400 "$(code -b "$JAR" -X PUT "$BASE/api/v1/departments/$DEPT5_ID/members" -H 'Content-Type: application/json' \
  --data-raw '{"users":["smoke-user5-not-exist"]}')" "不存在的成员 → 400"
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/departments/$DEPT5_ID/members" -H 'Content-Type: application/json' \
  --data-raw '{"users":[]}')" "清空部门成员 → 200"

# 目录筛选
expect_code 400 "$(code -b "$JAR" "$BASE/api/v1/users?status=bogus")" "非法 status → 400"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users?status=active&mfa=false&department_id=$DEPT5_ID")" "组合筛选 → 200"

# 管理动作防呆（内置管理员受保护）
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users?keyword=$ADMIN&size=50")" "按关键词查管理员 → 200"
ADMIN_ID="$(list_id name "$ADMIN")"
expect_code 403 "$(code -b "$JAR" -X POST "$BASE/api/v1/users/$ADMIN_ID/disable")" "禁用内置管理员 → 403"
expect_code 403 "$(code -b "$JAR" -X PUT "$BASE/api/v1/users/$ADMIN_ID/admin" -H 'Content-Type: application/json' \
  --data-raw '{"is_admin":false}')" "取消内置管理员 → 403"
expect_code 403 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/users/$ADMIN_ID")" "删除内置管理员 → 403"

# 登录失败锁定 → 解除（核心链路）
for i in 1 2 3 4 5; do
  code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
    --data-raw "$(printf '{"institution_id":%s,"application":"%s","username":"%s","password":"bad-%s"}' "$ORG_ID" "$APP" "$TEST_USER5" "$i")" >/dev/null
done
expect_code 429 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"application":"%s","username":"%s","password":"bad-6"}' "$ORG_ID" "$APP" "$TEST_USER5")")" "连续失败 5 次后登录被限制 → 429"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/users/$USER5_ID/security")" "GET 安全概览（含锁定条目）→ 200"
expect_ok "锁定条目标记 blocked" "
import json
items = json.load(open('/tmp/polaris-smoke-body'))['data']['lockouts']
print('OK' if items and items[0].get('blocked') else 'lockouts=%r' % items)"
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/users/$USER5_ID/unlock")" "一键解除限制 → 200"
expect_code 200 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"username":"%s","password":"%s"}' "$ORG_ID" "$TEST_USER5" 'Smoke5@12345')")" "解锁后可立即登录 → 200"

# 权限边界：普通用户不得重置他人 MFA / 解除锁定（user:reset_mfa / user:unlock 独立授权）
JAR5="$(mktemp "${TMPDIR:-/tmp}/polaris-smoke5.XXXXXX")"
expect_code 200 "$(code -c "$JAR5" -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"username":"%s","password":"%s"}' "$ORG_ID" "$TEST_USER5" 'Smoke5@12345')")" "普通用户登录 → 200"
expect_code 403 "$(code -b "$JAR5" -X POST "$BASE/api/v1/users/$ADMIN_ID/reset-mfa")" "普通用户重置他人 MFA → 403"
expect_code 403 "$(code -b "$JAR5" -X POST "$BASE/api/v1/users/$ADMIN_ID/unlock")" "普通用户解除他人锁定 → 403"

expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/job-titles/$GROUP5_ID")" "删除冒烟职务 → 200"
GROUP5_ID=""
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/departments/$DEPT5_ID")" "删除冒烟部门 → 200"
DEPT5_ID=""
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/users/$USER5_ID")" "删除阶段五用户 → 200"
USER5_ID=""

step "11. 角色 / 权限 / 职务关联"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/roles" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟角色"}' "$ORG_ID" "$TEST_ROLE")")" "创建角色 → 201"
ROLE_ID="$(json_field id)"
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/roles/$ROLE_ID/relations" -H 'Content-Type: application/json' \
  --data-raw '{"permissions":["permission-self"],"users":[]}')" "设置角色关联 → 200"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/roles/$ROLE_ID")" "删除角色 → 200"

step "11b. 部门（租户内部门树，含移动）"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/departments" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"name":"%s","display_name":"冒烟部门"}' "$TEST_DEPT")")" "创建部门 → 201"
DEPT_ID="$(json_field id)"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/departments" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"name":"%s","display_name":"冒烟子部门","parent_id":%s}' "$TEST_DEPT_CHILD" "$DEPT_ID")")" "创建子部门 → 201"
DEPT_CHILD_ID="$(json_field id)"
CHILD_LEVEL="$(json_field level)"
if [ "$CHILD_LEVEL" != "1" ]; then red "FAIL  子部门 level 应为 1，实际 $CHILD_LEVEL"; exit 1; fi
green "PASS  子部门层级 = $CHILD_LEVEL"
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/departments/$DEPT_CHILD_ID" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"display_name":"冒烟子部门","parent_id":null}')")" "移动子部门到顶层 → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/departments/$DEPT_CHILD_ID")" "查询子部门 → 200"
MOVED_LEVEL="$(json_field level)"
if [ "$MOVED_LEVEL" != "0" ]; then red "FAIL  移动后 level 应为 0，实际 $MOVED_LEVEL"; exit 1; fi
green "PASS  移动后层级 = $MOVED_LEVEL"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/departments/$DEPT_CHILD_ID")" "删除子部门 → 200"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/departments/$DEPT_ID")" "删除部门 → 200"

step "12. 应用与密钥（标识 code + 自动生成凭据 + 登录方式）"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications?institution=$ORG_ID&size=200")" "GET /applications（定位内置应用）→ 200"
BUILTIN_APP_ID="$(list_id code "$APP")"
if [ -z "$BUILTIN_APP_ID" ]; then red "FAIL  未找到内置应用 id"; exit 1; fi
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications/$BUILTIN_APP_ID")" "查询内置应用（按 id）→ 200"
# 客户端 ID / 密钥不再由调用方传入：请求体只给 code / name / 登录方式等策略字段
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/applications" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"code":"%s","name":"冒烟应用","signin_methods":[{"name":"Password"}],"self_signup_enabled":true,"signup_auto_signin":true,"grant_types":["authorization_code","refresh_token","password"],"redirect_uris":["%s"],"cert":"cert-built-in"}' "$ORG_ID" "$TEST_APP" "$REDIRECT_URI")")" \
  "创建测试应用 → 201"
TEST_APP_ID="$(json_field id)"
CLIENT_ID="$(json_field client_id)"
CLIENT_SECRET="$(json_field client_secret)"
if [ -z "$CLIENT_ID" ] || [ -z "$CLIENT_SECRET" ]; then red "FAIL  未能取得测试应用凭证"; exit 1; fi
green "PASS  自动生成客户端凭证 (client_id=$CLIENT_ID)"

# 安全回归：列表/详情响应绝不能下发密钥明文
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications/$TEST_APP_ID")" "查询测试应用 → 200"
if grep -q '"client_secret"' /tmp/polaris-smoke-body; then
  red "FAIL  详情接口下发了 client_secret"; exit 1
fi
if ! grep -q '"has_client_secret":true' /tmp/polaris-smoke-body; then
  red "FAIL  详情接口应以下发 has_client_secret 表达密钥存在性"; exit 1
fi
green "PASS  详情接口未下发密钥明文"

# 回归：应用是"标识(code)与名称(name)分离"的资源，保存时名称不得被标识覆盖。
# 历史 bug：handler 把路径 id 解析出的标识写进了 Name，导致每次保存都把「名称」变成标识。
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"name":"Net Bird Smoke","signin_methods":[{"name":"Password"}],"self_signup_enabled":true,"signup_auto_signin":true,"grant_types":["authorization_code","refresh_token","password"],"redirect_uris":["%s"],"cert":"cert-built-in"}' "$REDIRECT_URI")")" \
  "更新应用名称 → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications/$TEST_APP_ID")" "回读应用 → 200"
UPDATED_NAME="$(json_field name)"
UPDATED_CODE="$(json_field code)"
if [ "$UPDATED_NAME" != "Net Bird Smoke" ]; then
  red "FAIL  名称被覆盖成 ${UPDATED_NAME}（应保留用户填写的名称）"; exit 1
fi
if [ "$UPDATED_CODE" != "$TEST_APP" ]; then
  red "FAIL  标识被改动：${UPDATED_CODE}"; exit 1
fi
green "PASS  名称与标识各自独立（name=${UPDATED_NAME} code=${UPDATED_CODE}）"

step "12b. 配置即生效：登录方式驱动登录页"
expect_code 200 "$(code "$BASE/api/v1/auth/app-info?institution=$ORG&application=$TEST_APP")" "GET /auth/app-info → 200"
# 回归：应用驱动登录页按 client_id 拉 app-info 时，**组织标识字段必须是 institution（蛇形）**：
# 曾写成驼峰 institutionID，前端读 info.institution 永远 undefined，
# 于是登录页虽然渲染出了组织名/Logo，却始终不把组织带进登录与授权请求。
APPINFO_BY_CLIENT="$(curl -sS "$BASE/api/v1/auth/app-info?client_id=$CLIENT_ID")"
case "$APPINFO_BY_CLIENT" in
  *"\"institution_id\":$ORG_ID"*) green "PASS  app-info（按 client_id）带组织主键 institution_id=$ORG_ID" ;;
  *) red "FAIL  app-info 未下发组织主键: $(echo "$APPINFO_BY_CLIENT" | head -c 200)"; exit 1 ;;
esac
case "$APPINFO_BY_CLIENT" in
  *"\"institution\":\"$ORG\""*) green "PASS  app-info 兼容下发组织标识 institution=$ORG" ;;
  *) red "FAIL  app-info 未按契约下发组织标识: $(echo "$APPINFO_BY_CLIENT" | head -c 200)"; exit 1 ;;
esac
case "$APPINFO_BY_CLIENT" in
  *institutionID*) red "FAIL  app-info 出现驼峰字段 institutionID（前端读不到）"; exit 1 ;;
esac
if ! grep -q '"name":"Password"' /tmp/polaris-smoke-body; then
  red "FAIL  app-info 未下发已启用的登录方式 Password"; exit 1
fi
if grep -q '"name":"MagicLink"' /tmp/polaris-smoke-body; then
  red "FAIL  未启用的 MagicLink 不应下发"; exit 1
fi
green "PASS  登录方式按配置下发"
# 未启用魔法链接：发送接口必须拒绝（否则就是"配了不生效"的反面——没配却生效）
expect_code 403 "$(code -X POST "$BASE/api/v1/auth/magic-link" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution":"%s","application":"%s","email":"%s@example.com"}' "$ORG" "$TEST_APP" "smoke")")" \
  "未启用魔法链接时发送 → 403"

step "12c. 托管注册（self_signup_enabled 生效）"
expect_code 201 "$(code -X POST "$BASE/api/v1/auth/signup" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution":"%s","application":"%s","name":"%s","email":"%s@example.com","password":"Smoke-pass-123"}' "$ORG" "$TEST_APP" "$TEST_SIGNUP_USER" "$TEST_SIGNUP_USER")")" \
  "自助注册 → 201"
# 关闭自助注册后必须立刻拒绝（配置即生效）
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"code":"%s","name":"冒烟应用","signin_methods":[{"name":"Password"}],"self_signup_enabled":false,"grant_types":["authorization_code","refresh_token","password"],"redirect_uris":["%s"]}' "$ORG_ID" "$TEST_APP" "$REDIRECT_URI")")" \
  "关闭自助注册 → 200"
expect_code 403 "$(code -X POST "$BASE/api/v1/auth/signup" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution":"%s","application":"%s","name":"%s-b","email":"%s-b@example.com","password":"Smoke-pass-123"}' "$ORG" "$TEST_APP" "$TEST_SIGNUP_USER" "$TEST_SIGNUP_USER")")" \
  "关闭后注册 → 403"
curl -sS -b "$JAR" -o /tmp/polaris-smoke-body "$BASE/api/v1/users?keyword=$TEST_SIGNUP_USER"
SIGNUP_USER_ID="$(list_id name "$TEST_SIGNUP_USER")"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/users/${SIGNUP_USER_ID}")" "清理注册用户 → 200"

step "12d. 配置即生效：应用级登录失败限制"
# 应用级覆盖平台默认（5 次 / 5 分钟）：限制 2 次失败、等待 1 分钟
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"code":"%s","name":"冒烟应用","signin_methods":[{"name":"Password"}],"self_signup_enabled":false,"grant_types":["authorization_code","refresh_token","password"],"redirect_uris":["%s"],"signin_error_limit":2,"signin_retry_wait_minutes":1}' "$ORG_ID" "$TEST_APP" "$REDIRECT_URI")")" \
  "配置应用级登录失败限制（2 次 / 1 分钟）→ 200"
expect_code 401 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(login_body "$ORG" "$ADMIN" 'wrong-password' "$TEST_APP")")" "第 1 次错误密码 → 401"
expect_code 401 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(login_body "$ORG" "$ADMIN" 'wrong-password' "$TEST_APP")")" "第 2 次错误密码 → 401"
expect_code 429 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(login_body "$ORG" "$ADMIN" 'wrong-password' "$TEST_APP")")" "达到应用级上限（2 次）→ 429"
# 复位为 0 = 沿用平台默认（随后用例不再受该阈值影响；计数键按「组织+应用+账号+IP」隔离）
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"code":"%s","name":"冒烟应用","signin_methods":[{"name":"Password"}],"self_signup_enabled":false,"grant_types":["authorization_code","refresh_token","password"],"redirect_uris":["%s"],"signin_error_limit":0,"signin_retry_wait_minutes":0}' "$ORG_ID" "$TEST_APP" "$REDIRECT_URI")")" \
  "复位登录失败限制（0 = 平台默认）→ 200"

step "12e. 应用访问授权（谁能使用该应用）"
# 默认「全员可访问」：读到的模式是 all，三类名单是空集合（不是 null）
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications/$TEST_APP_ID/access")" "GET /applications/:id/access → 200"
expect_ok "默认 access_mode=all 且三类名单为空集合" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
ok = d.get('access_mode') == 'all' and d.get('users') == [] and d.get('groups') == [] and d.get('departments') == []
print('OK' if ok else '实际 %r' % d)"

# 可授权主体候选：**按组织**取（应用管理员通常没有 user/group/department 读权限）
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications/subject-options?institution=$ORG_ID&type=user&offset=0&limit=5")"   "GET /applications/subject-options（用户候选，分页）→ 200"
expect_ok "扁平候选下发 total（分页器数据源）" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
print('OK' if isinstance(d.get('total'), int) else '实际 %r' % d)"

# 部门候选是**树**：不带 parent_id 只返回顶层，带 parent_id 才取该节点的下一层（弹窗按层懒加载）
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/departments" -H 'Content-Type: application/json' --data-raw "$(printf '{"name":"%s","display_name":"准入父部门"}' "$TEST_ACCESS_DEPT")")"   "创建准入测试部门（父）→ 201"
ACCESS_DEPT_ID="$(json_field id)"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/departments" -H 'Content-Type: application/json' --data-raw "$(printf '{"name":"%s","display_name":"准入子部门","parent_id":%s}' "$TEST_ACCESS_DEPT_CHILD" "$ACCESS_DEPT_ID")")"   "创建准入测试部门（子）→ 201"
ACCESS_DEPT_CHILD_ID="$(json_field id)"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications/subject-options?institution=$ORG_ID&type=department&limit=50")"   "部门候选：顶层 → 200"
expect_ok "顶层不含子部门，且带 id / has_child（前端据此画树）" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
names = [i['name'] for i in d['items']]
parent = [i for i in d['items'] if i['name'] == '$TEST_ACCESS_DEPT']
ok = '$TEST_ACCESS_DEPT_CHILD' not in names and parent and parent[0].get('has_child') is True and parent[0].get('id')
print('OK' if ok else '实际 %r' % d)"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications/subject-options?institution=$ORG_ID&type=department&parent_id=$ACCESS_DEPT_ID&limit=50")"   "部门候选：展开父部门（按层取子）→ 200"
expect_ok "展开后只含它的直接子部门" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
names = [i['name'] for i in d['items']]
print('OK' if names == ['$TEST_ACCESS_DEPT_CHILD'] else '实际 %r' % names)"
# parent_id 只对部门有意义，且与关键词搜索互斥：一律显式拒绝（不静默忽略）
expect_code 400 "$(code -b "$JAR" "$BASE/api/v1/applications/subject-options?institution=$ORG_ID&type=user&parent_id=$ACCESS_DEPT_ID")"   "parent_id 用于非部门类型 → 400"
expect_code 400 "$(code -b "$JAR" "$BASE/api/v1/applications/subject-options?institution=$ORG_ID&type=department&parent_id=$ACCESS_DEPT_ID&keyword=x")"   "parent_id 与 keyword 互斥 → 400"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications/subject-options?institution=$ORG_ID&type=department&keyword=准入子部门")"   "部门关键词搜索（跨层级扁平结果）→ 200"
expect_ok "搜索结果带层级路径（关键词模式下不按层）" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
hit = [i for i in d['items'] if i['name'] == '$TEST_ACCESS_DEPT_CHILD']
print('OK' if hit and '准入父部门' in (hit[0].get('path') or '') else '实际 %r' % d)"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/departments/$ACCESS_DEPT_CHILD_ID")" "删除准入测试子部门 → 200"
ACCESS_DEPT_CHILD_ID=""
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/departments/$ACCESS_DEPT_ID")" "删除准入测试父部门 → 200"
ACCESS_DEPT_ID=""

expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/users" -H 'Content-Type: application/json' \
  --data-raw "$(create_user_body "$ORG_ID" "$TEST_ACCESS_USER" 准入用户 "smoke-access-$RUN_ID@polaris.local" 'Smoke@12345')")" \
  "创建准入测试用户 → 201"
ACCESS_USER_ID="$(json_field id)"
JAR_A="$(mktemp "${TMPDIR:-/tmp}/polaris-smoke-a.XXXXXX")"

# 受限 + 空名单：**允许保存**（Q4），该应用暂时无人可访问
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID/access" -H 'Content-Type: application/json' \
  --data-raw '{"access_mode":"restricted","users":[],"groups":[],"departments":[]}')" \
  "设置受限 + 空名单 → 200（不做拦截）"
expect_code 403 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"client_id":"%s","username":"%s","password":"Smoke@12345"}' "$ORG_ID" "$CLIENT_ID" "$TEST_ACCESS_USER")")" \
  "受限应用（空名单）带 client_id 登录 → 403"
# 判定作用域（Q6）：不带应用上下文的登录 = 登录平台，不做应用准入判定
# （正因如此，误配永远不会把管理员挡在控制台之外）
expect_code 200 "$(code -c "$JAR_A" -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"username":"%s","password":"Smoke@12345"}' "$ORG_ID" "$TEST_ACCESS_USER")")" \
  "不带应用上下文的平台登录 → 200（平台登录不判定）"

# 加入名单后放行（login 请求通过 client_id 携带应用上下文）
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID/access" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"access_mode":"restricted","users":["%s"],"groups":[],"departments":[]}' "$TEST_ACCESS_USER")")" \
  "加入授权名单 → 200"
expect_code 200 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"client_id":"%s","username":"%s","password":"Smoke@12345"}' "$ORG_ID" "$CLIENT_ID" "$TEST_ACCESS_USER")")" \
  "名单内用户带 client_id 登录 → 200"

# 回归：应用主表单是整行 UPDATE，**不得**重置访问模式（否则受限被静默放宽成全员可访问）
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"name":"Net Bird Smoke","signin_methods":[{"name":"Password"}],"self_signup_enabled":true,"signup_auto_signin":true,"grant_types":["authorization_code","refresh_token","password"],"redirect_uris":["%s"]}' "$REDIRECT_URI")")" \
  "更新应用主表单（不带 access_mode）→ 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications/$TEST_APP_ID/access")" "回读访问授权 → 200"
expect_ok "主表单更新后访问模式仍是 restricted" "
import json
d = json.load(open('/tmp/polaris-smoke-body'))['data']
print('OK' if d.get('access_mode') == 'restricted' else 'access_mode=%r（被静默放宽）' % d.get('access_mode'))"

# 列表「访问范围」列：access_mode + 授权主体数
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/applications?institution=$ORG_ID&size=200")" "GET /applications（列表）→ 200"
expect_ok "列表按应用下发 access_mode 与 access_subject_count" "
import json
items = json.load(open('/tmp/polaris-smoke-body'))['data']['items']
target = [a for a in items if a.get('code') == '$TEST_APP']
print('OK' if target and target[0].get('access_mode') == 'restricted' and target[0].get('access_subject_count') == 1 else '实际 %r' % target)"

# 未知主体一律拒绝（不静默忽略：否则会出现"界面上加了、运行时不生效"）
expect_code 400 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID/access" -H 'Content-Type: application/json' \
  --data-raw '{"access_mode":"restricted","users":["no-such-user-smoke"]}')" \
  "未知主体 → 400（INVALID_PARAMS）"

# 作为 IdP：未授权用户访问 authorize 必须按 OAuth2 规范回跳 error=access_denied
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID/access" -H 'Content-Type: application/json' \
  --data-raw '{"access_mode":"restricted","users":[],"groups":[],"departments":[]}')" \
  "移出名单（受限 + 空名单）→ 200"
AUTHORIZE_LOCATION="$(curl -sS -o /dev/null -w '%{redirect_url}' -b "$JAR_A" \
  "$BASE/oauth/authorize?client_id=$CLIENT_ID&redirect_uri=$REDIRECT_URI&response_type=code&scope=openid&state=smoke-denied")"
case "$AUTHORIZE_LOCATION" in
  *"error=access_denied"*)
    green "PASS  未授权用户的 authorize → 回跳 error=access_denied（${AUTHORIZE_LOCATION}）" ;;
  *)
    red "FAIL  authorize 未按 OAuth2 规范回跳拒绝：$AUTHORIZE_LOCATION"; exit 1 ;;
esac

# 复位为「全员可访问」：后续用例（password grant / 授权码流程）沿用默认语义
expect_code 200 "$(code -b "$JAR" -X PUT "$BASE/api/v1/applications/$TEST_APP_ID/access" -H 'Content-Type: application/json' \
  --data-raw '{"access_mode":"all","users":[],"groups":[],"departments":[]}')" \
  "复位为全员可访问 → 200"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/users/$ACCESS_USER_ID")" "删除准入测试用户 → 200"
ACCESS_USER_ID=""

step "13. 密码模式下发令牌（OAuth2 password grant）"
expect_code 200 "$(code -X POST "$BASE/oauth/token" \
  --data-urlencode grant_type=password --data-urlencode "client_id=$CLIENT_ID" --data-urlencode "client_secret=$CLIENT_SECRET" \
  --data-urlencode "username=$ADMIN" --data-urlencode "password=$PASSWORD" --data-urlencode 'scope=openid profile email')" \
  "password grant → 200"
ACCESS_TOKEN="$(json_top access_token)"
if [ -z "$ACCESS_TOKEN" ]; then red "FAIL  未取得 access_token"; exit 1; fi
expect_code 200 "$(code -H "Authorization: Bearer $ACCESS_TOKEN" "$BASE/oauth/userinfo")" "userinfo → 200"

step "14. 授权码流程"
expect_code 302 "$(code -b "$JAR" "$BASE/oauth/authorize?client_id=$CLIENT_ID&redirect_uri=$REDIRECT_URI&response_type=code&scope=openid%20profile&state=smoke-state")" \
  "授权端点 → 302（携带 code）"

step "15. 应用密钥轮换与清理"
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/applications/$TEST_APP_ID/reset-secret")" "重置测试应用密钥 → 200"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/applications/$TEST_APP_ID")" "删除测试应用 → 200"

step "16. 登出与会话失效"
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/auth/logout")" "登出 → 200"
expect_code 401 "$(code -b "$JAR" "$BASE/api/v1/users")" "登出后访问 → 401"

step "17. 应用信息（登录页公开端点）"
# 上一步已登出，这里重新建立管理会话（后续步骤仍需鉴权）
expect_code 200 "$(code -c "$JAR" -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(login_body "$ORG" "$ADMIN" "$PASSWORD" "$APP")")" "重新登录 → 200"
expect_code 200 "$(code "$BASE/api/v1/auth/app-info?institution=$ORG&application=$APP")" "GET /auth/app-info → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/providers/catalog")" "GET /providers/catalog（目录 schema 单一来源）→ 200"

step "18. 组织聚合统计"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/institutions")" "GET /institutions → 200"
ORG_ID="$(list_id name "$ORG")"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/institutions/$ORG_ID/stats")" "GET /institutions/:id/stats → 200"
STATS_TOTAL="$(json_field total)"
if [ -z "$STATS_TOTAL" ] || [ "$STATS_TOTAL" = "0" ]; then red "FAIL  组织统计 total 异常: $STATS_TOTAL"; exit 1; fi
green "PASS  组织资源合计 = $STATS_TOTAL"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/institutions/stats")" "GET /institutions/stats → 200"

step "19. 提供商多类别：对象存储（local）"
STORAGE_ROOT="/tmp/polaris-smoke-storage-$RUN_ID"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟存储","category":"storage","type":"local","state":"active","config":{"endpoint":"%s","domain":"%s"}}' "$ORG_ID" "$TEST_STORAGE" "$STORAGE_ROOT" "$BASE")")" \
  "创建 对象存储 提供商（config 契约）→ 201"
STORAGE_ID="$(json_field id)"
TEST_PROVIDERS="$STORAGE_ID"
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers/$STORAGE_ID/test")" "Storage 连通性自检 → 200"

# 单例类别（邮件/短信/人机校验/存储）同一组织最多一个启用项：
# 否则"上传到底用哪个存储"只能靠名称排序碰运气（迁移 028 已加部分唯一索引兜底）
expect_code 409 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","category":"storage","type":"local","state":"active","config":{"endpoint":"%s"}}' "$ORG_ID" "$TEST_STORAGE2" "$STORAGE_ROOT")")" \
  "单例类别第二个启用项 → 409（避免'到底用哪个'的歧义）"

# 先停用原生效项，再启用新的：切换是显式动作，且状态端点不动其它字段
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers/$STORAGE_ID/state" -H 'Content-Type: application/json' \
  --data-raw '{"state":"disabled"}')" "停用存储提供商 → 200"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟存储2","category":"storage","type":"local","state":"active","config":{"endpoint":"%s","domain":"%s"}}' "$ORG_ID" "$TEST_STORAGE2" "$STORAGE_ROOT" "$BASE")")" \
  "停用后可启用另一个存储提供商 → 201"
STORAGE2_ID="$(json_field id)"
TEST_PROVIDERS="$TEST_PROVIDERS $STORAGE2_ID"
# 停用态的提供商不得再被运行时选中（此处切换回启用会 409，正好验证约束）
expect_code 409 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers/$STORAGE_ID/state" -H 'Content-Type: application/json' \
  --data-raw '{"state":"active"}')" "已有一个生效项时切回启用 → 409"

printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00\x90wS\xde\x00\x00\x00\x0cIDATx\x9cc\xf8\xcf\xc0\x00\x00\x03\x01\x01\x00\x18\xdd\x8d\xb0\x00\x00\x00\x00IEND\xaeB`\x82' > /tmp/polaris-avatar.png
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/me/avatar" -F "file=@/tmp/polaris-avatar.png")" "上传头像 → 200"
AVATAR_URL="$(json_field avatar)"
if [ -z "$AVATAR_URL" ]; then red "FAIL  头像地址为空"; exit 1; fi
green "PASS  头像地址 = $AVATAR_URL"
expect_code 200 "$(code "$AVATAR_URL")" "通过静态路由读取头像 → 200"

step "20. 提供商多类别：验证码（本地图形）"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟验证码","category":"captcha","type":"image","state":"active"}' "$ORG_ID" "$TEST_CAPTCHA")")" \
  "创建 人机校验 提供商 → 201"
CAPTCHA_ID="$(json_field id)"
TEST_PROVIDERS="$TEST_PROVIDERS $CAPTCHA_ID"
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers/$CAPTCHA_ID/test")" "Captcha 自检 → 200"
expect_code 200 "$(code "$BASE/api/v1/auth/captcha?institution=$ORG")" "GET /auth/captcha → 200"
CAPTCHA_REQUIRED="$(json_field required)"
if [ "$CAPTCHA_REQUIRED" != "True" ]; then red "FAIL  验证码提供商启用后应要求校验，实际 required=$CAPTCHA_REQUIRED"; exit 1; fi
green "PASS  验证码已生效（required=${CAPTCHA_REQUIRED}）"

step "21. 验证码强制校验（启用后拒绝无验证码登录）"
expect_code 400 "$(code -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data-raw "$(login_body "$ORG" "$ADMIN" "$PASSWORD" "$APP")")" "无验证码登录 → 400"

step "22. 提供商多类别：邮件（SMTP 不可达时应如实报错）"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟邮件","category":"email","type":"smtp","state":"active","config":{"host":"127.0.0.1","port":9,"client_id":"noreply@example.com"},"secrets":{"client_secret":"smtp-secret"}}' "$ORG_ID" "$TEST_EMAIL")")" \
  "创建 邮件 提供商（密钥走 secrets 桶）→ 201"
EMAIL_ID="$(json_field id)"
TEST_PROVIDERS="$TEST_PROVIDERS $EMAIL_ID"
expect_code 503 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers/$EMAIL_ID/test" )" "SMTP 不可达 → 503（不伪装成功）"

step "22a. 短信提供商：模板变量按声明装配（阿里云对变量名严格匹配）"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟短信","category":"sms","type":"http","state":"disabled","config":{"endpoint":"http://127.0.0.1:9/send","sign_name":"Polaris","template_code":"SMS_1","template_params":["code","minutes"]}}' "$ORG_ID" "$TEST_SMS")")" \
  "创建 短信 提供商（声明模板变量 code+minutes）→ 201"
SMS_ID="$(json_field id)"
TEST_PROVIDERS="$TEST_PROVIDERS $SMS_ID"
SMS_VARS="$(json_field config | grep -o 'template_params.*' | head -1)"
case "$SMS_VARS" in
  *'code'*'minutes'*) green "PASS  模板变量已落库并回显：$SMS_VARS" ;;
  *) red "FAIL  模板变量未回显: $SMS_VARS"; exit 1 ;;
esac
# 变量名必须在内置变量集内：取不到值的名字在落库前就拒绝（否则会变成"配了但发送失败"）
expect_code 400 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s-bad","category":"sms","type":"http","state":"disabled","config":{"endpoint":"http://127.0.0.1:9/send","template_code":"SMS_1","template_params":["code","not_a_var"]}}' "$ORG_ID" "$TEST_SMS")")" \
  "未知模板变量 → 400（内置变量集之外的取值拒绝落库）"

step "22b. 外部身份提供方：控制台配置后登录页立即可见（无需重启）"
OIDC_NAME="smoke-oidc-$RUN_ID"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/providers" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟 OIDC","category":"oauth","type":"oidc","state":"active","config":{"issuer_url":"https://idp.example.com","client_id":"polaris-smoke","scopes":["openid","email"],"enable_pkce":true},"secrets":{"client_secret":"oidc-secret"},"policy":{"auto_create":true,"link_by_email":true,"allowed_domains":["example.com"]}}' "$ORG_ID" "$OIDC_NAME")")" \
  "创建 OIDC 提供商（策略走 policy 桶）→ 201"
OIDC_ID="$(json_field id)"
TEST_PROVIDERS="$TEST_PROVIDERS $OIDC_ID"
# 响应不得回发明文密钥，只以 has_secret 表达
OIDC_BODY="$(curl -sS -b "$JAR" "$BASE/api/v1/providers/$OIDC_ID")"
case "$OIDC_BODY" in
  *oidc-secret*) red "FAIL  提供商详情泄漏明文密钥"; exit 1 ;;
esac
case "$OIDC_BODY" in
  *'"has_secret":true'*) green "PASS  密钥只以 has_secret 表达" ;;
  *) red "FAIL  缺少 has_secret 标记: $OIDC_BODY"; exit 1 ;;
esac
# 语义（第五阶段验收期修复）：**应用没有配置提供方 = 该应用没有第三方登录入口**。
# 曾经的实现把"空列表"当成"全都要"，于是没配钉钉的应用（net-bird）登录页出现了钉钉按钮。
expect_code 200 "$(code "$BASE/api/v1/auth/providers?institution_id=$ORG_ID&application=$APP")" \
  "GET /auth/providers（未配置提供方的应用）→ 200"
PROVIDERS_BODY="$(curl -sS "$BASE/api/v1/auth/providers?institution_id=$ORG_ID&application=$APP")"
case "$PROVIDERS_BODY" in
  *"$OIDC_NAME"*) red "FAIL  未配置提供方的应用不得下发任何第三方入口: $PROVIDERS_BODY"; exit 1 ;;
  *'"providers":[]'*) green "PASS  未配置提供方的应用，第三方入口为空" ;;
  *) red "FAIL  未配置提供方时响应形态异常: $PROVIDERS_BODY"; exit 1 ;;
esac
# 明确指定了不存在的应用 → fail-closed（不得退化成"列出组织内全部提供方"）
expect_code 200 "$(code "$BASE/api/v1/auth/providers?institution_id=$ORG_ID&application=not-exist-$RUN_ID")" \
  "GET /auth/providers（未知应用）→ 200"
UNKNOWN_APP_BODY="$(curl -sS "$BASE/api/v1/auth/providers?institution_id=$ORG_ID&application=not-exist-$RUN_ID")"
case "$UNKNOWN_APP_BODY" in
  *"$OIDC_NAME"*) red "FAIL  未知应用不得下发第三方入口: $UNKNOWN_APP_BODY"; exit 1 ;;
  *) green "PASS  未知应用 fail-closed（不泄露组织内提供方）" ;;
esac

# 应用显式配置该提供方后，登录页立即可见（无需重启），且必须带 type/icon
TEST_LOGIN_APP="smoke-login-$RUN_ID"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/applications" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"code":"%s","name":"冒烟登录应用","grant_types":["password"],"signin_methods":[{"name":"Password"},{"name":"Provider"}],"providers":[{"name":"%s","canSignIn":true}]}' "$ORG_ID" "$TEST_LOGIN_APP" "$OIDC_NAME")")" \
  "创建显式配置提供方的应用 → 201"
LOGIN_APP_ID="$(json_field id)"
LOGIN_APP_CLIENT_ID="$(json_field client_id)"
expect_code 200 "$(code "$BASE/api/v1/auth/providers?institution_id=$ORG_ID&application=$TEST_LOGIN_APP")" \
  "GET /auth/providers（白名单应用）→ 200"
PROVIDERS_BODY="$(curl -sS "$BASE/api/v1/auth/providers?institution_id=$ORG_ID&application=$TEST_LOGIN_APP")"
# 回归：公开提供方列表必须带 type/icon —— 登录页据此渲染**品牌 logo**（此前只有名称，只能出文字按钮）
case "$PROVIDERS_BODY" in
  *'"type":"oidc"'*'"icon":"oidc"'*) green "PASS  登录页提供方带类型与图标（type/icon）" ;;
  *) red "FAIL  提供方列表缺少 type/icon: $PROVIDERS_BODY"; exit 1 ;;
esac
# client_id 驱动登录页：同样按应用白名单解析，且无需重启即生效（第四阶段之前读的是启动期静态注册表）
CLIENT_PROVIDERS="$(curl -sS "$BASE/api/v1/auth/providers?client_id=$LOGIN_APP_CLIENT_ID")"
case "$CLIENT_PROVIDERS" in
  *"$OIDC_NAME"*) green "PASS  client_id 驱动登录页可解析到该提供方（无需重启）" ;;
  *) red "FAIL  client_id 解析提供方失败: $CLIENT_PROVIDERS"; exit 1 ;;
esac
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/applications/$LOGIN_APP_ID?institution_id=$ORG_ID")" \
  "删除冒烟登录应用 → 200"
case "$PROVIDERS_BODY" in
  *"$OIDC_NAME"*) green "PASS  控制台新建的身份提供方对登录页立即可见（无需重启）" ;;
  *) red "FAIL  身份提供方未生效: $PROVIDERS_BODY"; exit 1 ;;
esac

step "23. 应用账号供给（连接器 / 账号策略 / 应用账号 / 供给历史）"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/connectors/types")" "GET /connectors/types → 200"
expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/connectors" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟连接器","application":"%s","type":"http","state":"active","config":{"provider_url":"http://127.0.0.1:9","principal":"svc"},"secrets":{"credentials":"secret"}}' "$ORG_ID" "$TEST_CONNECTOR" "$APP")")" \
  "创建连接器 → 201"
CONNECTOR_ID="$(json_field id)"
# 断言形状与实现无关：只看响应体里是否出现 has_secrets.credentials=true（且不含明文）
CONNECTOR_BODY="$(cat /tmp/polaris-smoke-body)"
case "$CONNECTOR_BODY" in
  *'"has_secrets":{"credentials":true}'*) green "PASS  连接器凭证以 has_secrets 表达" ;;
  *) red "FAIL  连接器应以 has_secrets 表达凭证存在，实际 $CONNECTOR_BODY"; exit 1 ;;
esac
case "$CONNECTOR_BODY" in
  # 明文回显（"credentials":"secret"）才算泄漏；has_secrets 这个键名本身不算
  *'"credentials":"secret"'*) red "FAIL  连接器响应泄漏明文凭证"; exit 1 ;;
esac
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/connectors")" "GET /connectors → 200"
expect_code 503 "$(code -b "$JAR" -X POST "$BASE/api/v1/connectors/$CONNECTOR_ID/test")" "下游不可达 → 503（不伪装成功）"
# 对齐检查（试运行）：无应用账号时报告为空，仍应 200
expect_code 200 "$(code -b "$JAR" -X POST "$BASE/api/v1/connectors/$CONNECTOR_ID/reconcile" -H 'Content-Type: application/json' --data-raw '{"dry_run":true}')" "POST /connectors/:id/reconcile → 200"

expect_code 201 "$(code -b "$JAR" -X POST "$BASE/api/v1/account-strategies" -H 'Content-Type: application/json' \
  --data-raw "$(printf '{"institution_id":%s,"name":"%s","display_name":"冒烟策略","application":"%s","is_enabled":true,"policy":{"createWhen":"manual","revokeWhen":"disable"},"mapping":{"email":"mail"}}' "$ORG_ID" "$TEST_STRATEGY" "$APP")")" \
  "创建账号策略 → 201"
STRATEGY_ID="$(json_field id)"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/account-strategies")" "GET /account-strategies → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/application-accounts")" "GET /application-accounts → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/provisioning")" "GET /provisioning → 200"
expect_code 200 "$(code -b "$JAR" "$BASE/api/v1/me/application-accounts")" "GET /me/application-accounts → 200（门户只读）"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/connectors/$CONNECTOR_ID")" "删除连接器 → 200"
expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/account-strategies/$STRATEGY_ID")" "删除账号策略 → 200"

step "24. 清理测试资源"
for PID in ${TEST_PROVIDERS}; do
  expect_code 200 "$(code -b "$JAR" -X DELETE "$BASE/api/v1/providers/$PID")" "删除提供商 id=$PID → 200"
done
TEST_PROVIDERS=""

green "\n全部冒烟用例通过 ✅"
