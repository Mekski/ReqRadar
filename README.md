# ReqRadar

*Watchlist-first hiring intelligence — a radar for when your target companies open SWE internship applications.*

ReqRadar tracks a personal watchlist of your target companies and answers the question job boards don't: **when does each company historically open its summer SWE-intern applications, and what's open right now?** It ingests live and historical job-posting signals, resolves them to your watchlist, and pushes sub-minute Telegram alerts the moment a new role opens.

It's built as an event-driven, multi-service system — the distributed-systems / real-time-data / CI-CD work that internship JDs ask for, not five cron scripts.

## What it does

- **Tiered watchlist** — bookmark companies into S/A/B/C tiers; add, remove, and re-rank from the dashboard.
- **"Expected to open"** — per company, the historical seasonal pattern of the summer SWE-intern cohort → an expected-open month plus a month-by-month chart.
- **Real-time alerts** — Telegram pings the moment a watchlist company opens a new role (sub-minute detection-to-alert, instrumented), plus a broad "firehose" of new SWE/AI-ML internships at non-watchlist companies.
- **~3 years of history** — backfilled by mining the SimplifyJobs aggregator's git history.
- **Dashboard** — a Next.js app: tiered watchlist, per-company detail, and the firehose feed.

## Architecture

Three Go services over NATS JetStream, one PostgreSQL, and a Next.js dashboard. Single VM + Docker Compose — deliberately no Kafka, no second database, no k8s (see [DESIGN.md](DESIGN.md) §10).

```
collectors ──signals.raw.*──▶ processor ──events.*──▶ api ──▶ dashboard + Telegram
(poll + git backfill)   (normalize → resolve → dedupe → Postgres)   (REST + alert dispatcher)
```

- **collector** — a plugin framework; sources fetch/stamp/hash only (SimplifyJobs listings + git-history backfill, Greenhouse, and Ashby today; HN planned).
- **processor** — normalize → entity-resolution cascade → dedupe/diff → Postgres → emit `events.*`. Idempotent; safe to replay.
- **api** — REST for the dashboard + the Telegram alert dispatcher (records `detect_to_alert_ms`; recency-gated so backfills never flood).

## Stack

Go 1.26 · NATS JetStream · PostgreSQL (time-partitioned) · Next.js 16 + TypeScript + Tailwind · Docker Compose · GitHub Actions CI (unit + integration).

## Run locally

Needs Docker, Go 1.26, and Node. Put secrets in `.env` (gitignored): `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, and `GITHUB_TOKEN` (for backfill).

```sh
make dev-up          # postgres, nats (+JetStream), prometheus, grafana
make migrate         # apply schema
make seed            # load seed/watchlist.yaml
make run-collector   # poll sources → NATS
make run-processor   # consume → Postgres + events
make run-api         # REST API + Telegram dispatcher (:8080)
make backfill        # ~3 years of history (run the processor alongside)
make firehose-prime  # arm the firehose once (so the backlog doesn't all alert)
cd web && npm install && npm run dev   # dashboard at http://localhost:3000
```

## Status

End-to-end and verified on a live stack (CI green): the collector → NATS → processor → Postgres pipeline, ~3 years of backfilled timing, watchlist + firehose Telegram alerts, three collectors (SimplifyJobs + git-history backfill, Greenhouse, Ashby), posted-pay extraction from ATS JDs, "expected open" SWE-intern seasonality across 30 companies, and a Next.js dashboard. Two on-demand LLM features run on a free-tier model: **fit score** (resume ↔ JD, rubric-calibrated) and a **grounded-search company sentiment** card. Remaining: a free always-on deployment so alerts run 24/7, plus the HN collector.

## Docs

- **[DESIGN.md](DESIGN.md)** — full architecture, data model, event flow, entity resolution, and rejected alternatives.
- **[WATCHLIST.md](WATCHLIST.md)** — verified per-company source mapping.
- **[CLAUDE.md](CLAUDE.md)** — current state, hard-won gotchas, and ranked next steps.
