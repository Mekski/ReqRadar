# Progress audit — 2026-06-15

A brutally-honest review of ReqRadar requested by Mark: what design choices are weak, where the security issues are, and what the next steps should be. Scope: the Go backend (collector/processor/api), the data model, CI/CD, secrets handling, and the Next.js dashboard.

## Verdict

The **architecture and the writing around it are genuinely strong** — DESIGN §10 (rejected alternatives) is exactly the kind of material that wins interviews, and the event-driven pipeline is real and defensible at the stated volume. The weakness at audit time was a **gap between what the docs claimed and what the code did**, concentrated in two areas Mark already suspected: **CI/CD was a facade, and there were zero tests.** Both have since been substantially addressed (see [changelog](../changelog.md)); this document records the state *as found* on 2026-06-15.

## What's good (leave alone)

- **3 services + NATS over a monolith**, with the "why not simpler?" objection pre-empted in DESIGN §2/§10. Right call, well-defended.
- **Postgres-only**; ClickHouse/Kafka/Redis/vector-DB each evaluated and rejected with sound reasoning.
- **`event_time` vs `observed_at` split**, partition-axis reasoning, idempotent replay, emoji/volatile-field-canonicalized content hashing (the job-watch lesson).
- **SQL safety**: every query in `internal/store/` is parameterized — zero string-built SQL. Verified.
- **Collector panic isolation**: per-source `recover` in `internal/collector/runner.go` genuinely delivers "one source dying can't stop the others."
- **Legal bright-lines** section is more thoughtful than most production systems have.

## What was wrong (found 2026-06-15)

### Critical / interview-risk
1. **Zero tests, CI structured to hide it.** `find -name '*_test.go'` → 0 files across 19 packages; `go test ./...` passed vacuously. DESIGN §7 calls golden-file tests "the highest-value test money in the system." → **Addressed**: see changelog (unit + golden + integration tests added).
2. **CI was lint-and-compile only.** `.github/workflows/ci.yml` ran `go vet`/`go build`/`go test` (the last a no-op). No linter, no frontend build, no integration test, no deploy. → **Partly addressed**: lint + frontend build + Postgres/NATS integration job added; deploy half still open (depends on host choice).
3. **Docs claimed auth that the code did not implement.** DESIGN §3.3 and CLAUDE.md claim "the API checks a static bearer token" and "Caddy basic-auth fronts it" — neither exists in code (`internal/api/server.go` has no auth; CORS is `*`). → **Open** (see [issues/audit-findings.md](../issues/audit-findings.md), SEC-1). Decision: handle at deploy time.

### Real bugs (alert delivery)
4. **The alert-loss trio (H1/H2/H3)** — three ways a genuinely-new posting's alert can be silently dropped or stalled. Not data corruption; alert-delivery reliability. Detailed in [issues/alert-loss-trio.md](../issues/alert-loss-trio.md).

### Lower severity
5. Telegram client ignores `resp.StatusCode` / `retry_after`; `detect_to_alert_ms` clamps negatives silently; `/healthz` isn't a real readiness check. See [issues/audit-findings.md](../issues/audit-findings.md).

## Design choices that are questionable (not bugs)

- **`posting_closed` deferral is now load-bearing debt.** Because it's deferred, `postings.status` is always `'open'`, so "open roles" is inflated everywhere, and the dashboard's flagship "when do apps open / are they open now" framing is partly blocked on it.
- **Scope creep risk.** DESIGN §1 warns over-engineering reads as junior, yet breadth (firehose, Calendar page, more collectors, app tracker) is being built while the §9 cut line says "CI/CD and tests are the resume; the rest is garnish." The garnish was getting built before the resume was proven. (Test/CI work since has narrowed this gap.)

## Security summary

- No SQL injection; secrets correctly gitignored (`.env` not tracked, not in history); no hardcoded secrets in client code; no `dangerouslySetInnerHTML` / XSS surface in the dashboard.
- **Open**: the API has no auth and CORS is wildcard `*` with mutating methods (POST/PATCH/DELETE). Localhost-only today, so low live risk — but the docs claim protection that doesn't exist. Must be real before any public deploy. (SEC-1)

## Next steps

See [plans/roadmap.md](../plans/roadmap.md) for the prioritized list.
