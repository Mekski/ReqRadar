# Changelog

Notable changes, newest first. Scoped to the audit-and-hardening workstream (the main feature history lives in git / CLAUDE.md). Commit hashes are short SHAs on `main`.

## 2026-06-15

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
