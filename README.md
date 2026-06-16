<div align="center">

# ReqRadar

**Watchlist-first hiring intelligence — a radar for when your target companies open SWE-internship applications.**

[![CI](https://github.com/Mekski/ReqRadar/actions/workflows/ci.yml/badge.svg)](https://github.com/Mekski/ReqRadar/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Next.js](https://img.shields.io/badge/Next.js-16-000000?logo=nextdotjs&logoColor=white)
![Postgres](https://img.shields.io/badge/PostgreSQL-17-4169E1?logo=postgresql&logoColor=white)
![NATS](https://img.shields.io/badge/NATS-JetStream-27AAE1?logo=natsdotio&logoColor=white)
![License](https://img.shields.io/badge/use-personal%20project-555)

<img src="docs/assets/dashboard-watchlist.png" alt="ReqRadar watchlist dashboard" width="900">

</div>

---

Job boards tell you what's open. ReqRadar tells you **when each of your target companies historically opens its summer SWE-intern applications — and pings your phone the moment a new role drops.** You bookmark ~30 companies into tiers; it mines live and 3-year-historical posting signals, resolves them to your watchlist, learns each company's seasonal opening window, and fires **sub-minute Telegram alerts** on every new role.

It's deliberately built as an **event-driven, multi-service system** — the distributed-systems, real-time-data, API-design, and CI/CD work that internship JDs actually ask for, not five cron scripts. Every component is meant to survive the interview question *"why not something simpler?"* (the rejected alternatives are written down in [DESIGN.md §10](DESIGN.md)).

---

## Features

### Tiered watchlist + "expected to open"

Bookmark companies into **S / A / B / C** tiers and re-rank them inline. For each, ReqRadar aggregates historical `posting_opened` events by month-of-year to surface an **expected-open month** and a season chart — answering the question that actually drives intern recruiting timing. Data-derived where there's enough history; a researched, cited estimate (`≈ est.`) or honest `rolling` otherwise.

<div align="center"><img src="docs/assets/company-detail.png" alt="Company detail — expected-open seasonality + community sentiment" width="820"></div>

### Real-time Telegram alerts + firehose

A new role at a watchlist company pings your phone in **under a second from detection** (instrumented end-to-end; 607 ms verified). A separate **firehose** channel surfaces newly-posted SWE / AI-ML internships at companies *off* your watchlist — an early-warning net — with real posted dates and apply links. Both are recency-gated so backfills and bulk adds never flood you.

<div align="center"><img src="docs/assets/telegram-alert.png" alt="Telegram alerts from @ReqRadarBot" width="300"></div>

### Resume ↔ JD fit score *(LLM)*

Upload your résumé (PDF) and score it against any watchlist role's real job description: a rubric-calibrated **0–100** (Technical 40 / Experience 25 / Impact 15 / Eligibility 10 / ATS 10), the matched and missing skills, and concrete résumé tips. Runs on a **free-tier Gemini** model, on-demand only — never in the alert path — and every (résumé, JD) result is cached forever so each unique pair costs one call.

<div align="center"><img src="docs/assets/fit-score.png" alt="Fit score — résumé vs JD" width="820"></div>

### Grounded company sentiment *(LLM)*

A one-click report per company synthesizing what engineers actually say — prestige, culture (liked vs disliked), interview process and OA difficulty, intern pay + housing stipend, return-offer rates, and watch-outs. Built on **Gemini's grounded Google Search** (real citations, nothing scraped), with a prompt that's required to say *"not enough public information found"* rather than invent specifics.

<div align="center"><img src="docs/assets/sentiment-card.png" alt="Grounded company sentiment report" width="820"></div>

### Also

- **Posted-pay extraction** — parses pay ranges out of Greenhouse/Ashby JDs (conservative: shows nothing rather than wrong comp).
- **~3 years of history** — backfilled by mining the SimplifyJobs aggregator's git history.

---

## Architecture

Three Go services over NATS JetStream, one PostgreSQL, a Next.js dashboard. Single VM + Docker Compose — **no Kafka, no second datastore, no Kubernetes** (each evaluated and rejected in [DESIGN.md §10](DESIGN.md)).

```mermaid
flowchart LR
    subgraph src [Sources]
      direction TB
      A1[SimplifyJobs<br/>poll + 3yr git backfill]
      A2[Greenhouse]
      A3[Ashby]
    end

    src --> COL[collector<br/>fetch · stamp · hash]
    COL -->|signals.raw.*| NATS{{NATS JetStream}}
    NATS --> PROC[processor<br/>normalize → resolve → dedupe → persist]
    PROC --> PG[(PostgreSQL)]
    PROC -->|events.* via outbox| NATS
    NATS --> API[api<br/>REST + alert dispatcher]
    PG --> API
    API --> WEB[Next.js dashboard]
    API --> TG[Telegram]
    API -.->|fit / sentiment · on-demand| GEM[Gemini free-tier]
```

- **collector** — a plugin framework: each source only fetches, stamps, and content-hashes (canonicalizing volatile fields so unchanged re-posts don't re-alert). Adding a source is one file + one registration line.
- **processor** — normalize → an entity-resolution cascade (exact → alias → domain → cache) → dedupe / version-diff → Postgres → emit `events.*`. Idempotent: replaying the stream is always safe.
- **api** — the dashboard's REST API plus the Telegram alert dispatcher, recording `detect_to_alert_ms` per alert. The LLM features live here, reached on-demand — never on the hot path.

**Reliability the alert claim actually depends on:** events are published via a **transactional outbox** (staged in the same DB transaction, published inline with a relay backstop) so a broker hiccup can't silently drop an alert; consumers have **redelivery caps + backoff** so a poison message can't hot-loop; and a failed Telegram send **redelivers** instead of being swallowed.

---

## Engineering highlights

- **Event-driven, idempotent pipeline** with JetStream replay as a first-class workflow (re-process from stored raw signals after a parser fix).
- **Transactional outbox + consumer redelivery policy** — the dual-write and at-least-once delivery story, end to end.
- **Real CI** (GitHub Actions): `lint` (golangci-lint), `unit`, `frontend` (`next build`), and an `integration` job that spins **real Postgres + NATS** to exercise the dedupe state machine and the outbox.
- **Golden-file collector tests** from captured payloads — source-format drift fails CI instead of silently dropping data.
- **`event_time` vs `observed_at`** kept distinct throughout, so 3-year backfill and sub-minute latency coexist without conflating "when it happened" with "when we saw it."
- A written **design doc with rejected alternatives** and a tracked [`docs/`](docs/) audit trail — decisions you can defend, not just code.

---

## Tech stack

**Backend** — Go 1.26 · NATS JetStream · PostgreSQL 17 (time-partitioned, `golang-migrate`) · `log/slog`
**Frontend** — Next.js 16 · React 19 · TypeScript · Tailwind 4 · react-markdown
**AI** — Gemini (free-tier): fit scoring + grounded-search sentiment
**Infra** — Docker Compose · GitHub Actions · Telegram Bot API

---

## Run locally

Needs Docker, Go 1.26, and Node. Put secrets in `.env` (gitignored): `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, `GITHUB_TOKEN` (backfill), and `GEMINI_API_KEY` (the LLM features; everything else runs without it).

```sh
make dev-up          # postgres, nats (+JetStream), prometheus, grafana
make migrate         # apply schema
make seed            # load seed/watchlist.yaml
make run-collector   # poll sources → NATS
make run-processor   # consume → Postgres + events
make run-api         # REST API + Telegram dispatcher (:8080)
make backfill        # ~3 years of history (run the processor alongside)
make firehose-prime  # arm the firehose once (so the backlog doesn't all alert)

cd web && npm install && npm run dev   # dashboard → http://localhost:3000
```

Integration tests need a throwaway Postgres + NATS: `make test-integration` (see the `REQRADAR_TEST_*` env in the Makefile).

---

## Status

End-to-end and verified on a live stack, CI green: the collector → NATS → processor → Postgres pipeline; three collectors (SimplifyJobs + git-history backfill, Greenhouse, Ashby); ~3 years of backfilled timing and "expected open" seasonality across 30 companies; posted-pay extraction; watchlist + firehose Telegram alerts; both LLM features; and the Next.js dashboard. **Remaining:** a free always-on deployment so alerts run 24/7, an HN sentiment source, and a calendar view.

> Single-user project (built for my own Summer-2027 internship search). The schema leaves the multi-user seam but there are no signup/auth flows by design.

## Docs

- **[DESIGN.md](DESIGN.md)** — full architecture, data model, event flow, entity resolution, and rejected alternatives.
- **[CLAUDE.md](CLAUDE.md)** — current state, hard-won operational gotchas, and ranked next steps.
- **[docs/](docs/)** — audits, tracked issues, roadmap, and a changelog.
- **[WATCHLIST.md](WATCHLIST.md)** — verified per-company source mapping.
