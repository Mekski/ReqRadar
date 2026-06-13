# ReqRadar

**Watchlist-first hiring intelligence — radar for job reqs at your target companies.**

Bookmark ~15 target companies; ReqRadar aggregates per-company hiring signals (historical application-opening timing, currently-open postings, JD version diffs, posted pay ranges, engineering-blog/HN sentiment) and fires a Telegram alert within a minute of detecting a change.

An event-driven pipeline: Go collectors → NATS JetStream → processor (normalize, entity-resolve, diff) → Postgres → API + alert dispatcher → Telegram, with a Next.js dashboard. Single VM, Docker Compose.

## Documentation

- **[DESIGN.md](DESIGN.md)** — the locked v1 design (architecture, data model, event flow, entity resolution, observability, build order, rejected alternatives).
- **[WATCHLIST.md](WATCHLIST.md)** — verified per-company source mapping (which companies expose Greenhouse/Ashby APIs vs. ride the aggregator).
- **[CLAUDE.md](CLAUDE.md)** — build conventions and invariants for contributors (human or agent).

## Layout

```
cmd/            service entrypoints (collector, processor, api)
internal/       shared packages (signal envelope, collector framework, config)
migrations/     forward-only SQL migrations (golang-migrate)
deploy/         docker-compose + observability config
web/            Next.js dashboard (added in Milestone B)
```

## Local development

```sh
docker compose -f deploy/docker-compose.yml up -d   # postgres, nats, prometheus, grafana
go build ./...
```

Status: **Milestone A — pipeline spine.** See the build checklist in [DESIGN.md §9](DESIGN.md).
