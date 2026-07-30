.PHONY: dev-up migrate sqlc test build run

COMPOSE_FILE := ../../docker-compose.dev.yml

dev-up:
	docker compose -f $(COMPOSE_FILE) up -d --wait

migrate:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	go run github.com/pressly/goose/v3/cmd/goose@v3.27.3 -dir db/migrations postgres \
		"$${MIGRATION_DATABASE_URL:-$$DATABASE_URL}" up

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate

test:
	go test ./...

build:
	go build ./...

run:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	go run ./cmd/server
