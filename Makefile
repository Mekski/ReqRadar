.PHONY: dev-up dev-down nats-reset migrate migrate-down build test test-integration fmt vet check

# Bring the local dev stack up/down (postgres, nats).
dev-up:
	docker compose -f deploy/docker-compose.yml up -d

dev-down:
	docker compose -f deploy/docker-compose.yml down

# Recreate NATS, wiping its streams + durable consumers. Needed after changing
# consumer config (e.g. the redelivery policy in internal/bus): js.Subscribe
# binds to — does not update — an existing durable, so the processor/api would
# otherwise fail to start with a config mismatch. NATS has no mounted volume, so
# this is a clean reset; EnsureStreams reprovisions and the collector re-emits.
nats-reset:
	docker compose -f deploy/docker-compose.yml up -d --force-recreate nats

# Apply / roll back migrations against the dev DB (uses config defaults).
migrate:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

# Load seed/watchlist.yaml into the registry/watchlist/sources (idempotent).
seed:
	go run ./cmd/seed

# Run the collector service (polls enabled sources -> NATS).
run-collector:
	go run ./cmd/collector

# Run the processor service (consumes signals.raw.* -> Postgres + events.*).
run-processor:
	go run ./cmd/processor

# Run the api service (REST API + Telegram alert dispatcher).
run-api:
	go run ./cmd/api

# Run the Next.js dashboard dev server (needs the api running; http://localhost:3000).
run-web:
	cd web && npm run dev

# One-shot historical backfill (needs GITHUB_TOKEN; run the processor alongside).
backfill:
	go run ./cmd/backfill

# Arm the firehose: record the current active backlog silently (run once).
firehose-prime:
	go run ./cmd/firehose-prime

build:
	go build ./...

test:
	go test ./...

# Integration tests (real Postgres + NATS). Needs REQRADAR_TEST_DSN (db name must
# contain "test") and a throwaway NATS at REQRADAR_TEST_NATS_URL (default :4222).
test-integration:
	go test -tags=integration ./internal/processor/... ./internal/bus/...

fmt:
	gofmt -w .

vet:
	go vet ./...

# What CI runs.
check: vet build test
