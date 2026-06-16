# Roadmap — prioritized next steps

From the [2026-06-15 audit](../audits/2026-06-15-progress-audit.md). Ordering reflects the DESIGN §9 cut line: *"the pipeline, backfill, alerts, and CI/CD are the resume; the rest is garnish."* Prove the resume first; add features second.

## Done since the audit
- ✅ First test suite: unit + golden-file (collector) + Postgres/NATS integration (dedupe state machine). See [changelog](../changelog.md).
- ✅ CI made real: `lint` (golangci-lint v2) + `frontend` (`next build`) + `integration` jobs; actions bumped to Node 24.
- ✅ Alert-loss trio (H1/H2/H3): consumer redelivery caps + transactional outbox (hybrid inline+relay). See [issues/alert-loss-trio.md](../issues/alert-loss-trio.md).
- ✅ Alert-path tests + Telegram error handling (OBS-1).
- ✅ Over-engineering audit follow-ups: dispatcher last-hop drop fixed (§1b), `resolution_decisions` made a cache (§4.1). Remaining items (partitioning, dead code, duplication, Prometheus) tracked in [issues/audit-findings.md](../issues/audit-findings.md).

## Next (recommended order)

1. **Deploy + CD half** (DESIGN §9 #14–15). Completes "CI/**CD**": GHCR image build → SSH deploy to a VM → post-deploy smoke test (synthetic signal → Telegram < 60s). **Blocked on a host decision** (Oracle Always Free / Fly.io — Mark prefers free tiers). Pair with **SEC-1**: implement the bearer-token + Caddy basic-auth the docs already claim *before* exposing anything publicly.

2. **`posting_closed` detection** (DESIGN §3.2 deferral). Unblocks honest "open roles" counts and the dashboard's open/closed clarity; reconcile "seen this poll" vs stored open postings (a batch concern). Collision-free with current frontend/processor hot-path work.

3. **More collectors** (greenhouse/ashby/hn). Framework + factories exist; each is one package + a `r.Register` line + a normalizer + golden-file tests. (Greenhouse/Ashby pull whole boards incl. full-time — filter to internships.)

4. **Backfill refresh** (`make backfill`) once new companies are added — the "when do apps open" features are meaningless without historical timing for the newly-added companies.

## Deliberately deferred (do not build yet — DESIGN §9 Milestone C / non-goals)
Interview-tips, JD-optimization tips, LinkedIn recruiter discovery, X collector, Levels-style comp, multi-user auth, k8s. The application tracker is a planned Milestone-C extra, not v1 core.
