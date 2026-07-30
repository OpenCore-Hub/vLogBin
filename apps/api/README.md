# Platform API (Phase 0)

Multi-provider identity & billing platform control-plane skeleton: Provider /
Environment / Region / Cell domain model, PostgreSQL RLS tenant isolation,
Test/Live separation, API-key credentials, transactional outbox, append-only
audit, dual commerce domains.

## Run

```sh
# 1. Start PostgreSQL 16 (creates the platform_app role on first boot)
make dev-up          # docker compose -f ../../docker-compose.dev.yml up -d --wait

# 2. Configure
cp .env.example .env # defaults match docker-compose.dev.yml

# 3. Run (goose migrations are applied automatically at startup)
make run
```

## Smoke test

```sh
# Create a provider (auto test environment + initial test API key)
curl -s -X POST localhost:8080/v1/operator/providers \
  -H "Authorization: Bearer dev-operator-token" \
  -H "Content-Type: application/json" \
  -d '{"slug":"acme","name":"Acme Inc","home_region_code":"cn-shanghai"}'
# -> {"provider":{...},"environments":[...],"api_key":"pk_test_..."}  (key shown once)

# Use the key
curl -s localhost:8080/v1/whoami -H "Authorization: Bearer pk_test_..."
```

## Development

- `make sqlc` — regenerate the query layer (`internal/store/storegen`) from
  `db/queries/*.sql` (pinned sqlc v1.29.0)
- `make test` — unit + integration tests (integration tests use
  testcontainers; Docker required)
- `make build` — compile all packages

## Layout

- `cmd/server` — entrypoint (migrations → HTTP server + outbox relay)
- `internal/config` — env-var configuration
- `internal/domain` — lifecycle state machine, scopes, commerce domains
- `internal/keys` — API-key format (pk_test_/pk_live_, sha256 storage)
- `internal/tenant` — credential-derived tenant context
- `internal/store` — pgx pool, WithTenant/WithOperator tx helpers, goose migrations
- `internal/service` — business operations (state change + outbox + audit in one tx)
- `internal/httpapi` — chi router, auth middleware, v1 handlers
- `internal/outbox` — relay worker (pending → published)
- `internal/integration` — testcontainers acceptance tests (RLS, API)
- `db/migrations` — goose migrations (embedded in the binary)
- `db/queries` — sqlc queries
