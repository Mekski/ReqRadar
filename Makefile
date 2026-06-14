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
