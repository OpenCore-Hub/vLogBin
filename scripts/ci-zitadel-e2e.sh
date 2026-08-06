#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

COMPOSE="docker compose -f docker-compose.test.yml --env-file .env.test -p vlogbin-test"
API_PID=""
WEB_PID=""

cleanup() {
  if [[ -n "$WEB_PID" ]]; then kill "$WEB_PID" 2>/dev/null || true; fi
  if [[ -n "$API_PID" ]]; then kill "$API_PID" 2>/dev/null || true; fi
  $COMPOSE down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> Starting ZITADEL + platform DB"
$COMPOSE up -d --wait zitadel-db zitadel zitadel-login zitadel-proxy platform-db

CONSOLE_CLIENT_ID="$(
  docker exec vlogbin-test-zitadel-db-1 psql -U postgres -d zitadel -tAc \
    "select client_id from projections.apps7_oidc_configs limit 1"
)"
LOGIN_CLIENT_PAT="$(
  docker run --rm -v vlogbin-test_zitadel-bootstrap:/data alpine:3.20 \
    cat /data/login-client.pat
)"

export ZITADEL_URL="http://localhost:8080"
export ZITADEL_CONSOLE_CLIENT_ID="$CONSOLE_CLIENT_ID"
export ZITADEL_CONSOLE_REDIRECT_URI="http://localhost:8080/ui/console/auth/callback"
export ZITADEL_ADMIN_LOGIN="zitadel-admin@zitadel.localhost"
export ZITADEL_ADMIN_PASSWORD="Password1!"
export ZITADEL_E2E_REDIRECT_URI="http://localhost:3100/callback"
export ZITADEL_E2E_LOGIN_BASE_URI="http://localhost:3100/"

echo "==> Provisioning OIDC app"
PROVISION_OUTPUT="$(
  cd apps/web
  ZITADEL_LOGIN_CLIENT_PAT="$LOGIN_CLIENT_PAT" node scripts/zitadel-e2e-provision.mjs
)"
CLIENT_ID="$(printf '%s' "$PROVISION_OUTPUT" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).clientId))')"
CLIENT_SECRET="$(printf '%s' "$PROVISION_OUTPUT" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).clientSecret))')"

VAULT_KEY="$(openssl rand -hex 32)"
PSP_KEY="$(openssl rand -hex 32)"

echo "==> Starting platform API"
(
  cd apps/api
  HTTP_ADDR=":8085" \
  DATABASE_URL="postgres://platform_app:platform_app_dev@localhost:5432/platform?sslmode=disable" \
  MIGRATION_DATABASE_URL="postgres://postgres:postgres@localhost:5432/platform?sslmode=disable" \
  OPERATOR_TOKEN="test-operator-token" \
  AUTH_VAULT_SERVICE_TOKEN="ci-vault-token" \
  AUTH_VAULT_MASTER_KEY="$VAULT_KEY" \
  PSP_MASTER_KEY="$PSP_KEY" \
  PLATFORM_BASE_DOMAIN="platform.local" \
  BILLING_ADAPTER="noop" \
  CORS_ALLOWED_ORIGINS="*" \
  LOG_LEVEL="info" \
  ZITADEL_URL="http://localhost:8080" \
  go run ./cmd/server
) &
API_PID=$!

for i in $(seq 1 60); do
  if curl -fsS "http://localhost:8085/health" >/dev/null 2>&1; then break; fi
  sleep 1
done

echo "==> Building web standalone"
cd apps/web
pnpm run build

echo "==> Starting web standalone"
(
  cd .next/standalone
  AUTH_MODE="oidc-custom-login" \
  APP_BASE_URL="http://localhost:3100" \
  ZITADEL_URL="http://localhost:8080" \
  ZITADEL_API_URL="http://localhost:8080" \
  ZITADEL_CLIENT_ID="$CLIENT_ID" \
  ZITADEL_CLIENT_SECRET="$CLIENT_SECRET" \
  ZITADEL_LOGIN_CLIENT_PAT="$LOGIN_CLIENT_PAT" \
  SESSION_SECRET="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
  PLATFORM_API_URL="http://localhost:8085" \
  AUTH_VAULT_SERVICE_TOKEN="ci-vault-token" \
  PORT="3100" HOSTNAME="localhost" \
  node server.js
) &
WEB_PID=$!

for i in $(seq 1 60); do
  if curl -fsS "http://localhost:3100/login" >/dev/null 2>&1; then break; fi
  sleep 1
done

echo "==> Running browser E2E"
WAIT_CONSOLE="true" \
VLOGBIN_WEB_URL="http://localhost:3100" \
ZITADEL_ADMIN_LOGIN="$ZITADEL_ADMIN_LOGIN" \
ZITADEL_ADMIN_PASSWORD="$ZITADEL_ADMIN_PASSWORD" \
node scripts/zitadel-custom-login-e2e.mjs
