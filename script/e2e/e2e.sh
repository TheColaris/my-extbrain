#!/usr/bin/env bash
# my-extbrain E2E 洁净室：容器洁净库 + 三重门服务 + 旅程（纪律与机制见 script/e2e/README.md）。
# 用法：script/e2e/e2e.sh {up|reset|start|stop|down|journey|status}
#
# 环境事实：
#   容器   my-extbrain-pg-e2e（pgvector/pg16 → 宿主 5434，独立于 dev 5433）
#   洁净库 extbrain_e2e（reset 重建）
#   服务   http://127.0.0.1:8081（三重门 fail-closed：E2E_MODE + 库名 + 标记行）
#   顺序   up → reset → start（start 内部自动 bootstrap 建表 → seed 标记行 → 带门重启）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SRV="$ROOT/server"
DIR="$ROOT/script/e2e"
COMPOSE="$DIR/docker-compose.e2e.yml"
BIN="$DIR/bin/extbrain-server"
PID="$DIR/.server.pid"
LOG="$DIR/server.log"
VAPID_ENV="$DIR/.e2e-vapid.env"

PORT=8081
BASE="http://127.0.0.1:$PORT"
DB_HOST=127.0.0.1
DB_PORT=5434
DB_NAME=extbrain_e2e
DATABASE_URL="postgres://extbrain:extbrain@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"
INTERNAL_TOKEN="e2e-internal-token"
JWT_SECRET="e2e-cleanroom-jwt-secret-0123456789abcdef0123456789abcdef" # 仅洁净环境（≥32 位）

dc() { docker compose -f "$COMPOSE" "$@"; }

# 防误伤护栏：host 必须回环、库名必须洁净库（本脚本内的 DATABASE_URL 是唯一入口）
guard() {
  case "$DATABASE_URL" in
    postgres://*@127.0.0.1:*) ;;
    *) echo "✗ 护栏：DATABASE_URL host 必须是 127.0.0.1（防误伤远程库）" >&2; exit 1 ;;
  esac
  case "$DATABASE_URL" in
    *"/$DB_NAME"*) ;;
    *) echo "✗ 护栏：库名必须是 $DB_NAME" >&2; exit 1 ;;
  esac
}

wait_health() {
  for _ in $(seq 1 60); do
    if curl -fsS "$BASE/healthz" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  echo "✗ 服务健康检查超时（日志: $LOG 末 20 行）" >&2
  tail -20 "$LOG" >&2 || true
  exit 1
}

stop_server() {
  if [ -f "$PID" ]; then
    kill "$(cat "$PID")" >/dev/null 2>&1 || true
    for _ in $(seq 1 20); do kill -0 "$(cat "$PID")" 2>/dev/null || break; sleep 0.5; done
    rm -f "$PID"
  fi
}

# start_server [e2e] —— e2e=带三重门启动；否则普通启动（bootstrap 建表用）。
# env -i 清环境：防 shell 里游离的 E2E_MODE/DATABASE_URL 污染洁净判定。
start_server() {
  local with_e2e="${1:-}"
  local vpub="" vpriv=""
  if [ -f "$VAPID_ENV" ]; then
    # shellcheck disable=SC1090
    vpub="$(grep '^VAPID_PUBLIC_KEY=' "$VAPID_ENV" | cut -d= -f2-)"
    vpriv="$(grep '^VAPID_PRIVATE_KEY=' "$VAPID_ENV" | cut -d= -f2-)"
  fi
  local e2e_args=()
  [ "$with_e2e" = "e2e" ] && e2e_args=(E2E_MODE=true)
  (
    cd "$SRV"
    exec env -i \
      HOME="$HOME" PATH="$PATH" \
      DATABASE_URL="$DATABASE_URL" \
      LISTEN_ADDR=":$PORT" \
      JWT_SECRET="$JWT_SECRET" \
      REGISTER_ENABLED=true \
      MIGRATE_AUTO=true \
      MCP_ENABLED=true \
      INTERNAL_TOKEN="$INTERNAL_TOKEN" \
      DOWNLOADS_DIR="$DIR/downloads" \
      VAPID_PUBLIC_KEY="$vpub" \
      VAPID_PRIVATE_KEY="$vpriv" \
      ${e2e_args[@]+"${e2e_args[@]}"} \
      "$BIN" >>"$LOG" 2>&1
  ) &
  echo $! >"$PID"
}

ensure_bin() {
  if [ ! -x "$BIN" ]; then
    echo "==> 编译服务二进制（含 embed 前端）"
    (cd "$SRV" && go build -o "$BIN" ./cmd/app)
  fi
}

ensure_vapid() {
  if [ ! -f "$VAPID_ENV" ]; then
    echo "==> 生成洁净室 VAPID 密钥（Web Push 捕获链路用）"
    (cd "$SRV" && go run ./cmd/app -gen-vapid >"$VAPID_ENV")
  fi
}

# 洁净库不存在则建（让 start 可独立跑，不必先 reset）
ensure_db() {
  dc exec -T db psql -U extbrain -d extbrain -tAc \
    "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" | grep -q 1 || \
    dc exec -T db psql -U extbrain -d extbrain -c "CREATE DATABASE $DB_NAME;" >/dev/null
}

cmd="${1:-}"
case "$cmd" in
  up)
    guard
    dc up -d --wait
    echo "✓ 洁净库容器就绪（${DB_HOST}:${DB_PORT}）"
    ;;
  reset)
    guard
    stop_server
    dc up -d --wait
    dc exec -T db psql -U extbrain -d extbrain -c "DROP DATABASE IF EXISTS $DB_NAME;" >/dev/null
    dc exec -T db psql -U extbrain -d extbrain -c "CREATE DATABASE $DB_NAME;" >/dev/null
    echo "✓ 洁净库已重建：$DB_NAME"
    ;;
  start)
    guard
    ensure_bin
    ensure_vapid
    dc up -d --wait
    ensure_db
    # 第一段：无门启动（bootstrap）——迁移自动建表（0013 信箱表在内）
    echo "==> bootstrap：无门启动建表"
    stop_server
    start_server
    wait_health
    # 第二段：写门③标记行 + 邮件服务配置（幂等）
    # 邮件配置：注册验证码链路的 Ready 前置；key 为假值——E2E 模式下发信只落信箱、绝不外发
    dc exec -T db psql -U extbrain -d "$DB_NAME" -c \
      "INSERT INTO tp_system_config (config_key, config_value, is_secret) VALUES
         ('e2e_cleanroom_marker', '1', 0),
         ('email.enabled', 'true', 0),
         ('email.from_address', 'noreply@e2e.test', 0),
         ('email.from_name', 'E2E 洁净室', 0),
         ('email.resend_api_key', 're_e2e_cleanroom_fake', 1)
       ON CONFLICT (config_key) DO NOTHING;" >/dev/null
    # 第三段：带门重启（三重门全开）
    echo "==> 带三重门重启"
    stop_server
    start_server e2e
    wait_health
    echo "✓ 洁净服务就绪：${BASE}（三重门全开；日志 ${LOG}）"
    ;;
  stop)
    stop_server
    echo "✓ 服务已停（容器保留；彻底停库用 down）"
    ;;
  down)
    stop_server
    dc down
    echo "✓ 已停服并停容器（数据卷保留；连卷删用 destroy）"
    ;;
  destroy)
    # 销毁：删容器 + 删数据卷（不可逆；与 journey/单测流程完全独立，须显式单独执行）
    stop_server
    dc down -v --remove-orphans
    echo "✓ 已销毁：洁净库容器与数据卷已删除（下次 up 重新初始化）"
    ;;
  journey)
    guard
    ensure_bin
    ensure_vapid
    "$0" reset
    "$0" start
    echo "==> 跑旅程"
    if (cd "$DIR" && BASE_URL="$BASE" INTERNAL_TOKEN="$INTERNAL_TOKEN" node run-journey.mjs); then
      "$0" stop
    else
      echo "✗ 旅程失败（现场保留：服务 $BASE 仍在跑，日志 ${LOG}）" >&2
      exit 1
    fi
    ;;
  status)
    dc ps
    if curl -fsS "$BASE/healthz" >/dev/null 2>&1; then echo "✓ 服务在跑：$BASE"; else echo "服务未运行"; fi
    ;;
  *)
    echo "用法: $0 {up|reset|start|stop|down|destroy|journey|status}" >&2
    exit 1
    ;;
esac
