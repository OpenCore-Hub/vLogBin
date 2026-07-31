#!/usr/bin/env bash
set -euo pipefail

# Start the unified test environment (ZITADEL + Lago + Platform DB)
# Usage: ./scripts/test-env.sh [up|down|status]

COMPOSE_FILE="docker-compose.test.yml"
ENV_FILE=".env.test"
PROJECT_NAME="vlogbin-test"

cmd="${1:-up}"

case "$cmd" in
  up)
    echo "🚀 Starting unified test environment..."
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" up -d --wait
    echo ""
    echo "✅ All services are healthy!"
    echo ""
    echo "━━━ Service Endpoints ━━━"
    echo "  Platform DB:  localhost:5432 (postgres/postgres, db=platform)"
    echo "  ZITADEL:      http://localhost:8080"
    echo "  Lago API:     http://localhost:3000"
    echo "  Lago Front:   http://localhost:8081"
    echo ""
    echo "━━━ Platform API Configuration ━━━"
    echo "  DATABASE_URL=postgres://postgres:postgres@localhost:5432/platform?sslmode=disable"
    echo "  SUPERUSER_DATABASE_URL=postgres://postgres:postgres@localhost:5432/platform?sslmode=disable"
    echo "  BILLING_ADAPTER=lago"
    echo "  LAGO_API_URL=http://localhost:3000"
    echo "  OPERATOR_TOKEN=test-operator-token"
    echo "  PSP_MASTER_KEY=$(openssl rand -hex 32)"
    echo ""
    echo "━━━ Next Steps ━━━"
    echo "  1. Get Lago API key:"
    echo "     Open http://localhost:8081 → create account → get API key"
    echo "     Set LAGO_API_KEY=<your-key>"
    echo ""
    echo "  2. Start the platform API:"
    echo "     cd apps/api && go run ./cmd/server"
    echo ""
    echo "  3. Run integration tests:"
    echo "     make test-integration"
    ;;

  down)
    echo "🛑 Stopping test environment..."
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" down -v
    echo "✅ Stopped and volumes removed."
    ;;

  status)
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" ps
    ;;

  *)
    echo "Usage: $0 [up|down|status]"
    exit 1
    ;;
esac
