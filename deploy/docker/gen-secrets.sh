#!/usr/bin/env bash
# 生成 deploy/docker/.env：全部密钥用 openssl rand 强随机生成。
# 已存在 .env 时拒绝覆盖（FORCE=1 可覆盖）。
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="$DIR/.env"

if [ -f "$OUT" ] && [ "${FORCE:-0}" != "1" ]; then
  echo "ERROR: $OUT 已存在（FORCE=1 可覆盖）"
  exit 1
fi

rand() { openssl rand -base64 24 | tr -d '/+=' | cut -c1-"$1"; }

cat > "$OUT" <<EOF
# 由 gen-secrets.sh 生成于 $(date '+%F %T') —— 全部强随机，请妥善保管（勿入库）

# ---- MySQL ----
MYSQL_ROOT_PASSWORD=$(rand 24)
MYSQL_APP_PASSWORD=$(rand 24)
MYSQL_DATABASE=wendaoiot

# ---- 后端 HTTP ----
WQ_JWT_SECRET=$(rand 48)
WQ_CORS_ORIGINS=https://panel.wendaoiot.com
WQ_ADMIN_USERNAME=admin
WQ_ADMIN_PASSWORD=$(rand 16)

# ---- 独立 MQTT broker ----
MQTT_PORT=1883
MQTT_TLS_PORT=8883
WQ_BROKER_SERVER_USERNAME=wendao_server
WQ_BROKER_SERVER_PASSWORD=$(rand 24)
WQ_MQTT_CLIENT_ID=wendao_server
WQ_SYNC_SECRET=$(rand 32)

# ---- nginx ----
DOMAIN=panel.wendaoiot.com
WIKI_DOMAIN=wiki.wendaoiot.com
PORTAL_HTTP_PORT=8088
WIKI_ROOT=/opt/wendao/web/portal
EOF

chmod 600 "$OUT"
echo "OK: $OUT (mode 600)"
echo "管理员初始密码: $(grep WQ_ADMIN_PASSWORD "$OUT" | cut -d= -f2)"
