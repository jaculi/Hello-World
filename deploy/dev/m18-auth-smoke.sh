#!/bin/sh
# M18 认证链路冒烟脚本（在 compose 网络内运行）。
# 用法：
#   docker run --rm -i --network jsl-tenant-dev_default \
#     --entrypoint sh curlimages/curl -s < deploy/dev/m18-auth-smoke.sh
set -e

KEYCLOAK=http://keycloak:8080
GATEWAY=http://apisix:9080
TENANT_ULID=01JSM18DEM0TENANT000000001

TOKEN=$(curl -s -X POST "$KEYCLOAK/realms/jsl/protocol/openid-connect/token" \
  -d "grant_type=password" \
  -d "client_id=jsl-gateway" \
  -d "client_secret=dev-gateway-secret" \
  -d "username=demo-admin" \
  -d "password=dev" \
  | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')

echo "token length: ${#TOKEN}"
[ "$TOKEN" ] || { echo "FAIL: no token"; exit 1; }

code() { curl -s -o /dev/null -w "%{http_code}" "$@"; }

echo "=== 1) 无令牌访问 /api（预期 401）==="
c1=$(code "$GATEWAY/api/v1/tenants/$TENANT_ULID")
echo "HTTP $c1"; [ "$c1" = "401" ] || { echo "FAIL 1"; exit 1; }

echo "=== 2) 合法令牌（预期认证链通过；业务 404 证明已过 authz/租户头注入）==="
c2=$(code "$GATEWAY/api/v1/tenants/$TENANT_ULID" -H "Authorization: Bearer $TOKEN")
echo "HTTP $c2"; [ "$c2" = "404" ] || { echo "FAIL 2 (expect 404 not_found after auth)"; exit 1; }

echo "=== 3) 篡改签名令牌（预期 401）==="
c3=$(code "$GATEWAY/api/v1/tenants/$TENANT_ULID" -H "Authorization: Bearer ${TOKEN}xx")
echo "HTTP $c3"; [ "$c3" = "401" ] || { echo "FAIL 3"; exit 1; }

echo "=== 4) Keycloak 发现端点经网关代理（预期 200）==="
c4=$(code "$GATEWAY/realms/jsl/.well-known/openid-configuration")
echo "HTTP $c4"; [ "$c4" = "200" ] || { echo "FAIL 4"; exit 1; }

echo "ALL PASS"
