.PHONY: dev-up dev-down migrate migrate-down build test fmt vet check

# Bring the local dev stack up/down (postgres, nats, prometheus, grafana).
dev-up:
	docker compose -f deploy/docker-compose.yml up -d

dev-down:
	docker compose -f deploy/docker-compose.yml down

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

fmt:
	gofmt -w .

vet:
	go vet ./...

# What CI runs.
check: vet build test
