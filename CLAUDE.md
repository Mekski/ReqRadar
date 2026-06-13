# ReqRadar

Watchlist-first hiring intelligence ("radar for job reqs") for ~15 target companies: posting detection with 3 years of backfilled timing history, JD version diffing, posted-pay-range extraction, HN/Reddit sentiment summaries, sub-minute detection-to-alert via Telegram. Single user (Mark), resume project for Summer 2027 SWE internship recruiting.

**Read DESIGN.md before making any architectural change.** It is the locked design; §10 (rejected alternatives) lists decisions that must not be silently re-introduced.

## Locked stack

- **Go** for the three backend services: `collector`, `processor`, `api` (REST + Telegram alert dispatcher in one binary).
- **NATS JetStream** between services. Subjects: `signals.raw.<source>` (collector → processor), `events.<type>` (processor → consumers).
- **Postgres only.** No ClickHouse, no Redis, no vector DB — all evaluated and rejected (DESIGN.md §10). `raw_signals` and `events` are month-partitioned; raw kept 30 days, events/versions/resolution-decisions forever.
- **Next.js + TypeScript** dashboard, behind Caddy (TLS + basic auth). API also checks a static bearer token.
- **Claude API** for entity-disambiguation fallback (cached forever in `resolution_decisions`/`entity_aliases`) and per-(company, day) batched sentiment summaries. Never in the alert hot path.
- **Deploy:** single VM, Docker Compose, GitHub Actions → GHCR → SSH deploy → post-deploy smoke test. No k8s, no serverless.

## Core invariants

- Collectors fetch/stamp/hash only — **no semantic parsing in collectors**. All interpretation in `processor`, so fixes are replayed from stored raw signals (JetStream replay is a first-class workflow).
- `RawSignal` carries both `EventTime` (when it happened — backfill may be years past) and `ObservedAt` (when we saw it — t0 for latency). Never conflate them; timing analytics group by event_time.
- Processing is idempotent: pure function of (raw signal, DB state). Replays must always be safe.
- One source dying must never stop the others or the alert path (panic recovery per plugin, health flag, operator Telegram ping).
- Entity resolution cascade: exact → alias → domain → Claude (write-back alias) → NULL. Every outcome appended to `resolution_decisions`. One LLM call max per unique string, ever.
- Entity registry is person-aware (`kind` enum) even though v1 only stores companies — v2 recruiter discovery must be additive.
- `detect_to_alert_ms` is recorded on every alert; the claim is sub-minute **detection-to-alert**, never "from posting" (sources are polled).

## Legal bright lines (non-negotiable)

- Never scrape behind a login; never create/use accounts for scraping; never fabricate identities (hiQ v. LinkedIn lesson).
- Never scrape Levels.fyi. Comp data = posted pay ranges extracted from JDs only.
- No direct LinkedIn access of any kind. Recruiter enrichment is v2, pending ToS review of third-party APIs.
- Respect robots.txt and published API rate limits; per-source rate limiting via `golang.org/x/time/rate`.
- Reddit collector ships only after a hands-on read of current Reddit API terms confirms this use.

## Data sources (v1)

Verified source mapping is in WATCHLIST.md (probed live 2026-06-13). `simplify-listings` (poll listings.json + git-history Backfiller — epochs verified back to Aug 2023), `greenhouse` (Roblox/Anthropic/xAI/Riot/Epic) + `ashby` (OpenAI/Notion) — public JSON APIs, per-org config, cover 7/15 incl. 6 top targets, `rss` (eng blogs), `hn` (Firebase API; replaces X — X cut, no free tier since Feb 2026), `vanshb03` aggregator, `reddit` (viable but inference-only + deletable storage, see below). **Lever cut** — no watchlist company uses it. Big tech (Google/Meta/Apple/Microsoft/Amazon) + NVIDIA/EA detection rides the aggregator repos, not bespoke scrapers.

**Reddit constraints (from actual Data API Terms):** never train/fine-tune on Reddit content (inference-only sentiment via Claude is fine); Reddit-derived rows (raw + summaries) must be segregated into a deletable partition — §6 requires deletion on API termination, so they CANNOT go in the forever-retained bucket; OAuth + honest User-Agent + privacy-policy page + non-commercial only. HN is the primary sentiment source (no such baggage); Reddit is optional/deferrable.

## Conventions

- Adding a source = one file implementing the `Collector` interface (+ optional `Backfiller`) + a `sources` row. If it takes more than that, the framework regressed.
- Every collector has golden-file fixture tests from real captured payloads. Format drift must fail CI, not silently drop data.
- `log/slog` JSON logging; thread one signal ID through all services via NATS header. Prometheus metrics per service; Grafana dashboard is a demo artifact — keep it presentable.
- Migrations: `golang-migrate`, forward-only.
- Non-goals v1 (do not build): multi-user auth flows, browse/search UX, recruiter enrichment, X collector, Levels-style comp, k8s.

## Owner context

Mark reads every diff and must be able to defend every line and decision in interviews — prefer clear, idiomatic code over clever code, and when making a non-obvious choice, note the why in the PR/commit, not in code comments. Interview defensibility is a primary design requirement; "simplest thing that works, with the upgrade path documented" is the house style.
