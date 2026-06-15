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

> Related, still **open** — the dispatcher *swallows* send errors (logs + continues; `dispatcher.go` `handleWatchlist`/`handleFirehose`), so a Telegram outage still drops the alert rather than Nak-ing for redelivery. Now that the events consumer has `MaxDeliver`/`BackOff` (H3), returning the error could make failed sends retry — but that has multi-recipient duplicate implications, so it's left as a deliberate follow-up, not changed in this pass.

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
