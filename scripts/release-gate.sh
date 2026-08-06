#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

export GOCACHE="${GOCACHE:-/tmp/vlb-go-cache}"

echo "==> [1/7] API build + vet"
(cd apps/api && go build ./... && go vet ./...)

echo "==> [2/7] API unit tests"
(cd apps/api && sh -c 'go test $(go list ./... | grep -v "/internal/integration") -count=1')

echo "==> [3/7] API integration tests"
(cd apps/api && go test ./internal/integration -count=1)

echo "==> [4/7] Contract checks"
make contract

echo "==> [5/7] Official SDK tests (Go / TypeScript / Python)"
make sdk

echo "==> [6/7] Web static checks"
(cd apps/web && npx tsc --noEmit && npx eslint .)

echo "==> [7/7] Full E2E"
(cd apps/web && WEB_URL="${WEB_URL:-http://localhost:3002}" API_URL="${API_URL:-http://localhost:8084}" npx playwright test)

echo "Release gate PASSED"
