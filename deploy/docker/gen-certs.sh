# 生成 8883 TLS 自签 CA 与服务器证书（独立 broker 容器使用）。
# 用法（Git Bash / WSL / MSYS2）： ./gen-certs.sh
# 强制重新生成： FORCE_CERTS=1 ./gen-certs.sh
# SAN 主机名覆盖： CERT_HOSTS="panel.wendaoiot.com localhost" ./gen-certs.sh
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERTDIR="$DIR/certs"
DAYS_CA=3650
DAYS_SRV=1095

HOSTS="${CERT_HOSTS:-panel.wendaoiot.com localhost 127.0.0.1}"

if [ -f "$CERTDIR/server.crt" ] && [ "${FORCE_CERTS:-0}" != "1" ]; then
  echo "SKIP: $CERTDIR/server.crt 已存在（FORCE_CERTS=1 可重新生成）"
  openssl x509 -in "$CERTDIR/server.crt" -noout -subject -dates -ext subjectAltName 2>/dev/null || true
  exit 0
fi

mkdir -p "$CERTDIR"
TMP_EXT="$(mktemp)"
trap 'rm -f "$TMP_EXT"' EXIT

alt="subjectAltName="
first=1
for h in $HOSTS; do
  if [[ "$h" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    entry="IP:$h"
  else
    entry="DNS:$h"
  fi
  if [ $first -eq 1 ]; then alt+="$entry"; first=0; else alt+=",$entry"; fi
done
echo "$alt" > "$TMP_EXT"

echo "=== [1/3] CA ==="
openssl req -x509 -newkey rsa:2048 -nodes -days "$DAYS_CA" \
  -keyout "$CERTDIR/ca.key" -out "$CERTDIR/ca.crt" \
  -subj "/CN=WendaoIoT CA/O=WendaoIoT/C=CN"

echo "=== [2/3] server key/csr ==="
openssl req -newkey rsa:2048 -nodes \
  -keyout "$CERTDIR/server.key" -out "$CERTDIR/server.csr" \
  -subj "/CN=panel.wendaoiot.com/O=WendaoIoT/C=CN"

echo "=== [3/3] sign server cert ==="
openssl x509 -req -in "$CERTDIR/server.csr" \
  -CA "$CERTDIR/ca.crt" -CAkey "$CERTDIR/ca.key" -CAcreateserial \
  -out "$CERTDIR/server.crt" -days "$DAYS_SRV" -extfile "$TMP_EXT"
rm -f "$CERTDIR/server.csr"
chmod 600 "$CERTDIR/server.key" "$CERTDIR/ca.key"

echo
openssl verify -CAfile "$CERTDIR/ca.crt" "$CERTDIR/server.crt"
echo "certificates generated in: $CERTDIR"
echo "设备侧需导入: $CERTDIR/ca.crt"
