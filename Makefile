.PHONY: build test test-integration lint contract sqlc dev dev-down frontend docker-build test-env test-env-down clean

# ---- Go API ----

build:
	cd apps/api && go build ./...

test:
	cd apps/api && go test ./...

test-integration:
	cd apps/api && go test -timeout 180s ./internal/integration/

lint:
	cd apps/api && go vet ./...
	cd apps/web && npx tsc --noEmit

contract:
	./scripts/check-openapi-contracts.sh
	./scripts/check-asyncapi-contracts.sh

sqlc:
	cd apps/api && sqlc generate

# ---- Frontend ----

frontend:
	cd apps/web && npm run build

# ---- Docker ----

docker-build:
	docker build -t platform-api -f apps/api/Dockerfile apps/api/

# ---- Development Environment ----

dev:
	docker compose -f docker-compose.dev.yml up -d

dev-down:
	docker compose -f docker-compose.dev.yml down

# ---- Test Environment (ZITADEL + Lago + Platform DB) ----

test-env:
	./scripts/test-env.sh up

test-env-down:
	./scripts/test-env.sh down

# ---- Misc ----

clean:
	rm -f apps/api/platform-api
