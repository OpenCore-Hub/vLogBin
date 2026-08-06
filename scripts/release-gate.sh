#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

export GOCACHE="${GOCACHE:-/tmp/vlb-go-cache}"

echo "==> [1/6] API build + vet"
(cd apps/api && go build ./... && go vet ./...)

echo "==> [2/6] API unit tests"
(cd apps/api && sh -c 'go test $(go list ./... | grep -v "/internal/integration") -count=1')

echo "==> [3/6] API integration tests"
(cd apps/api && go test ./internal/integration -count=1)

echo "==> [4/6] Contract checks"
make contract

echo "==> [5/6] Official SDK tests"
(cd sdk/go && go test ./...)

echo "==> [6/6] Web static checks + full E2E"
(cd apps/web && npx tsc --noEmit && npx eslint .)
(cd apps/web && WEB_URL="${WEB_URL:-http://localhost:3002}" API_URL="${API_URL:-http://localhost:8084}" npx playwright test)

echo "Release gate PASSED"
