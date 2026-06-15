# Changelog

Notable changes, newest first. Scoped to the audit-and-hardening workstream (the main feature history lives in git / CLAUDE.md). Commit hashes are short SHAs on `main`.

## 2026-06-15

### Feature — Greenhouse collector (second source; proves the plugin framework)
Added `internal/collector/greenhouse` — the second collector, dropped in via one `r.Register("greenhouse", greenhouse.New)` line (the framework's whole point). Polls the public Greenhouse board API for every org in the source config and emits one signal per **internship** req.
- **Intern filter** (`isInternship`, word-boundary `\bintern(ship)?s?\b` on title OR department): live boards carry the company's whole req list — Anthropic's has 380 jobs, ~0 real interns. A naive substring match wrongly catches "**Intern**al Auditor" / "**Intern**ational"; the fixture pins all three cases. Filtering here is a volume guard (like simplify's active-only filter), not interpretation — the real normalization stays in the processor.
- **Processor:** `normalizeGreenhouse` derives terms (season-from-title, so `is_summer` works like SimplifyJobs) and a coarse category (`Software Engineering` / `AI/ML/Data`), and resolves company via `company_name` → org-slug-alias fallback. Registered in the normalizers map.
- **Robustness:** per-org conditional GET (ETag), per-org error isolation (one org's 500 doesn't drop the others' signals — only an all-orgs failure errors the run), `first_published` → EventTime, content/updated_at excluded from the content hash (Greenhouse rewrites body markup → would cause phantom jd_changed alerts).
- **No Backfiller:** the API only exposes open reqs, so Greenhouse contributes live detection + pay, not history.
- **Verified live:** 2 intern postings resolved to watchlist (Roblox, Epic), 0 full-time leaked, 0 alerts (old `first_published` → 48h freshness gate). Tests: golden parse, the word-boundary filter table, hash-volatility, toSignal, conditional-GET, partial/all-org failure; plus `normalizeGreenhouse`/`termsFromTitle`/`greenhouseCategory`.
- **Next:** pay-range extraction from the JD `content` HTML (`pay_input_ranges` is unused by these orgs), then the `ashby` collector.

### Alert-path hardening + tests (OBS-1)
The dispatcher and Telegram client — the product's payoff — were untested. Added focused coverage of their pure logic and fixed the Telegram error handling:
- **OBS-1 fix** (`internal/telegram/telegram.go`): `SendMessage` now checks `resp.StatusCode`, surfaces it (with HTTP status-text fallback for non-JSON 5xx bodies), and reports `parameters.retry_after` on a 429 — previously a 429/5xx produced an empty `"failed: "` error.
- **Tests:** `internal/telegram/telegram_test.go` (success + request shape, ok:false, 429+retry_after, non-JSON 5xx via an httptest server) and `internal/api/dispatcher_test.go` (`withinFreshness` 48h boundary, `shouldAlert` default/custom/empty/malformed config, `formatAlert` posting_opened vs jd_changed ± location/url, `formatFirehose`).
- Extracted the 48h freshness gate into `withinFreshness(eventTime, now)` so the boundary is testable without a clock.
- **Left as deliberate follow-up** (noted in [issues/audit-findings.md](issues/audit-findings.md)): the dispatcher swallows send errors instead of Nak-ing for redelivery — fixing it has multi-recipient duplicate implications.

### Feature — "expected open" curated-seed fallback + seed-drift reconciliation + dead-LLM cleanup
Shipped the sparse-company fallback for the dashboard's flagship "expected open" month (CLAUDE.md item 1a). The confidence gate (`attachExpected`, `minSamples=5`) leaves 11/20 companies without a data-derived month; those now show a **curated estimate** instead of "—".
- **Plumbing:** `expected_estimate` flows YAML → `cmd/seed` → `store.UpsertCompany` (into `entities.metadata`) → `CompanySummary` on `/api/companies` → `CompanyCard.tsx`. UI priority: data-month → estimate ("≈ est.") → "rolling" → "—".
- **Values (researched, cited in the YAML):** months Google→Oct, Riot→Sep, Databricks→Aug, GitHub→Sep, LinkedIn→Sep, Roblox→Jul; **"rolling"** for Anthropic/OpenAI/xAI/Notion/Niantic (no documented season — honest over a fabricated month). Source: deep-research workflow + targeted web search.
- **Seed drift reconciled:** `seed/watchlist.yaml` now mirrors the live watchlist (tiers aligned to dashboard edits; Tesla/TikTok/Databricks/GitHub/LinkedIn added), so `make seed` is reproducible and non-destructive. Verified the user's `telegram_chat_id` survives a re-seed. Gotcha documented: `make seed` does not auto-load `.env`.
- **Dead-code cleanup (Mark's request):** removed `ANTHROPIC_API_KEY` from `.env.example` and fixed a stale "LLM-estimate fallback" comment in `api.go`. Confirmed via grep that **no LLM client code ever existed** (no package, no HTTP calls, config never read the key) — nothing functional to remove. The free-tier-only LLM decision stands (CLAUDE.md LLM roadmap).
- Go tests + vet green; feature verified live against the running stack (all 20 cards show a meaningful value, data-vs-estimate priority correct).

### Fix H1 + H2 — transactional outbox (alert-loss trio)
The processor wrote to Postgres *and* published to NATS as a dual write; a publish failure after commit (H2) or after marking a firehose posting seen (H1) silently dropped the alert forever. Fixed with a **transactional outbox**:
- New `event_outbox` table (migration `000007`); events are staged in the **same transaction** as their posting/firehose writes.
- **Hybrid publishing:** publish inline right after commit (latency stays sub-second — the 607ms claim is preserved), mark the row published; a relay goroutine (`RelayOutbox`, every 2s in `cmd/processor`) resends any rows a failed inline publish left behind. At-least-once (a crash between publish and mark yields at most one duplicate, tolerated by the 48h freshness gate).
- `maybeFirehose` now runs `MarkFirehoseSeenTx` + `InsertOutbox` in one tx (closes H1). New store code isolated in `internal/store/outbox.go` to avoid touching `api.go` (concurrent edits).
- Tests: `TestProcessorStateMachine`/`TestProcessorFirehose` assert inline-published outbox rows; `TestProcessorOutboxRelay` asserts the backstop resends a straggler exactly once (no duplicate).
- **Design correction (second-opinion review):** an earlier plan made the relay the *only* publisher, which would have regressed latency to the relay tick. Switched to inline + relay-backstop before implementing.
- Follow-ups: Prometheus gauge on outbox backlog (once metrics infra lands); periodic prune of old published rows. See [issues/alert-loss-trio.md](issues/alert-loss-trio.md).

### Fix H3 — consumer redelivery cap (alert-loss trio)
`internal/bus/bus.go`: both JetStream consumers now set `MaxDeliver(5)`, `AckWait(30s)`, `BackOff([30s, 2m, 5m])`, so a poison message can't be redelivered in an infinite loop (CPU/log spin + head-of-line blocking). Added `internal/bus/bus_integration_test.go` to assert the created consumers carry the config, and wired it into the CI `integration` job.
- **Found via the test (not assumed):** when `BackOff` is set, NATS uses `BackOff[0]` as the ack deadline and overrides `AckWait`. With `BackOff[0]=5s` and the dispatcher's 10s Telegram timeout, a slow-but-successful send would be redelivered → duplicate alert. Fixed by setting `BackOff[0]=30s`.
- **Deploy note:** `js.Subscribe` binds to (does not update) an existing durable, so deploying this requires recreating the "processor"/"dispatcher" consumers. Added `make nats-reset` (dev) and `make test-integration`. See [issues/alert-loss-trio.md](issues/alert-loss-trio.md).
- H1 + H2 (transactional outbox) remain open.

### Documentation: `docs/` tree established
Added this `docs/` directory (audits / issues / plans + changelog) to track the audit, findings, and forward plan. See [docs/README.md](README.md).

### CI made real — `76176cb`
Expanded `.github/workflows/ci.yml` from vet/build/test toward the DESIGN §7 pipeline:
- **`lint`** job — golangci-lint v2.12.2 (standard linters: errcheck/govet/ineffassign/staticcheck/unused). `.golangci.yml` excludes errcheck only for conventional ignore-on-cleanup functions (deferred `Close`/`Rollback`, best-effort `ResponseWriter.Write`). 0 issues on current code, including `--build-tags=integration`.
- **`frontend`** job — `npm ci` + `next build`, closing the "a TS/build error ships to main silently" gap.
- Bumped `actions/checkout`→v6, `actions/setup-go`→v6, added `actions/setup-node`@v6 (Node 24 runtime) — cleared the Node 20 deprecation warning.
- All four jobs (`unit`, `lint`, `integration`, `frontend`) verified green in real GitHub Actions.

### First test suite — `aab5b14`
The repo had **zero tests**; `go test ./...` passed vacuously. Added (all additive, no production code changed):
- `internal/entity/normalize_test.go` — `Normalize`, 100% coverage.
- `internal/processor/resolve_test.go` — resolution cascade (alias/domain/precedence/miss/reload) + `hostOf`.
- `internal/processor/normalize_test.go` — `normalizeSimplify` parsing.
- `internal/collector/simplify/simplify_test.go` (+ real captured `testdata/listings_sample.json`) — golden-file parse (format-drift guard), the job-watch content-hash canonicalization lesson, and the full `Collect` HTTP path (active filter, conditional-GET/304, error status).
- `internal/processor/integration_test.go` — the dedupe/diff state machine (new→opened, idempotent re-delivery→touch, changed-hash→jd_changed, firehose dedup) against **real Postgres + NATS**, triple-guarded (`integration` build tag + `REQRADAR_TEST_DSN` whose db name must contain "test") so it cannot touch the dev database. Verified green locally and in CI.

### Audit — `docs/audits/2026-06-15-progress-audit.md`
Brutally-honest progress review. Headline gaps found: zero tests, CI-as-facade (both now addressed), and docs claiming auth not implemented in code (SEC-1, open).
