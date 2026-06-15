# Audit findings (non-trio)

Tracked findings from the [2026-06-15 progress audit](../audits/2026-06-15-progress-audit.md), excluding the alert-loss trio (see [alert-loss-trio.md](alert-loss-trio.md)). Severity and status maintained here.

## Security

### SEC-1 — API has no auth; CORS is wildcard with mutating methods
**Severity:** high (on deploy) · **Status:** open — deferred to deploy time by Mark (2026-06-15)

DESIGN §3.3 and CLAUDE.md claim "the API checks a static bearer token" and "Caddy basic-auth fronts it." Neither exists in code: [internal/api/server.go](../../internal/api/server.go) has no auth middleware, and `cors()` sets `Access-Control-Allow-Origin: *` while allowing `POST/PATCH/DELETE`. Anyone who can reach the API can mutate the watchlist from any origin. Localhost-only today, so low live risk — but the docs claim protection that doesn't exist.

**Fix options:** (a) implement the static bearer-token middleware the docs already promise + scope CORS to the real dashboard origin in prod, sending the token from the dashboard; or (b) if deferring, correct DESIGN/CLAUDE so they stop claiming auth that isn't there. Do not deploy publicly without (a).

## Correctness / robustness (low severity)

### OBS-1 — Telegram client ignores HTTP status and `retry_after`
**Status:** fixed (2026-06-15)
`SendMessage` now checks `resp.StatusCode`, surfaces it in the error, falls back to the HTTP status text when the body isn't JSON (the prior code produced an empty `"failed: "` on a 5xx), and surfaces `parameters.retry_after` on a 429. Verified by `internal/telegram/telegram_test.go` (success/request-shape, ok:false, 429+retry_after, non-JSON 5xx).

> Related — **fixed (2026-06-15).** The dispatcher previously *swallowed* send errors (logged + continued, then Ack'd), so a Telegram outage dropped the very alert the upstream transactional outbox works to preserve — the weak last hop a separate over-engineering audit rightly flagged as partly negating H1/H2. `handleWatchlist`/`handleFirehose` now return the send error, so the events consumer Nak's and redelivers it under the `MaxDeliver(5)`/`BackOff([30s,2m,5m])` caps from H3 (after which it's terminated + logged, not hot-looped). Single-user note: with one recipient per event a redelivery just retries the failed send; multi-user would re-notify already-sent recipients (bounded, acceptable) until alerts-table dedup is added.

### OBS-2 — `detect_to_alert_ms` clamps negatives to 0 silently
**Status:** open · cosmetic
[internal/api/dispatcher.go](../../internal/api/dispatcher.go) masks clock skew / future `ObservedAt` rather than logging it. Minor metric-integrity nit.

### OBS-3 — `/healthz` is liveness, not readiness
**Status:** open · cosmetic
[internal/api/server.go](../../internal/api/server.go) returns "ok" even if the DB pool is dead. Fine as liveness; misleading if used as readiness. Consider a `/readyz` that pings the pool.

## Frontend (dashboard)

These were noted while the dashboard was under active development by another agent; line references may have shifted. Re-verify before acting.

### FE-1 — prod API base URL falls back to localhost silently
**Status:** open
`web/lib/api.ts`: `API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080"`. In prod, an unset env var makes RSC fetches hit localhost server-side and fail quietly. Fix: fail loudly in prod when unset.

### FE-2 — array-index `key` on re-fetched lists
**Status:** open · low
Index keys on lists fetched with `cache: "no-store"` cause React mis-reconciliation on refresh. Key on a stable field.

### FE-3 — mutation handlers swallow errors / generic "API down" message
**Status:** open · low
Some handlers discard fetch errors and every failure renders "no connection to API," conflating 500s / parse errors with network failure. Surface real failures.

> Note: the audit also confirmed several things **clean**: no SQL injection, secrets gitignored, no XSS surface, sensible RSC/client split, current dependencies. Those need no tracking.

## Over-engineering audit (2026-06-15)

A second, separate audit focused on over-engineering. Verified accurate (~90%); one factual miss (it claimed `updateTier` was dead — it's called by `CompanyCard`). Items and status:

### OE-§1b — dispatcher dropped alerts at the last hop
**Status:** fixed (2026-06-15). The transactional outbox preserved events to NATS, but the dispatcher swallowed Telegram send failures and Ack'd anyway. `handleWatchlist`/`handleFirehose` now return the send error → Nak → redeliver under H3's caps. See [alert-loss-trio.md](alert-loss-trio.md) and the OBS-1 note above.

### OE-§4.1 — resolution_decisions re-appended per restart
**Status:** fixed (2026-06-15). Now a cache (UNIQUE `raw_text` + `ON CONFLICT DO NOTHING`); migration `000009` collapsed 36,847→4,258 dev rows. See changelog.

### OE-§1a — events/raw_signals month-partitioning is premature
**Status:** open · design call (yours). At ~1k–35k rows, partitioning is overhead (and forces composite PKs that block the `alerts`/`events` FKs). The stronger interview signal is the reverse: a plain indexed table + a documented "convert at N rows" migration. Reverting is a real (forward-only) schema change, so it's a deliberate decision, not a bug. Recommendation: revert to a plain table, keep the conversion migration documented — but it's defensible to keep if framed as a known premature bet.

### OE-§2 — dead surface
**Status:** open · deferred (collision). `CompanyTiming` + `/api/companies/{id}/timing` and `OpenPostings` + `/api/postings` have no frontend caller today (superseded by Seasonality); `postings_status_idx` indexes a constant; `pay_*` columns were speculative (now used by the Greenhouse/Ashby pay work). Lives in `api.go`/`server.go`/`web` — the other agent's active files; sweep once they're clear, and confirm the planned Calendar page won't reuse the timing endpoint before deleting it.

### OE-§3 — duplicated definitions
**Status:** open · deferred (collision). `firehoseCategories` is copied verbatim in `firehose.go` and `cmd/firehose-prime`; the SWE category list appears in ~3 shapes; `MarkFirehoseSeen`/`MarkFirehoseSeenTx` can collapse (DBTX subsumes the pool variant). Mechanical; do in one sweep when the backend files are free.

### OE-§5 — Prometheus/Grafana run but nothing is instrumented
**Status:** open. Compose runs both; zero `promhttp`/metrics in Go. Either instrument (`detect_to_alert_ms`, outbox backlog are the obvious gauges) or drop the containers until then. Pairs with the metrics follow-up already noted for the outbox relay.
