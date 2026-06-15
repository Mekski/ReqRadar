# ReqRadar project docs

Working documentation that complements the two top-level design files:

- **[DESIGN.md](../DESIGN.md)** — the locked v1 architecture and rejected alternatives (§10). Source of truth for *why the system is shaped this way*.
- **[CLAUDE.md](../CLAUDE.md)** — current build status, run order, and operational gotchas. Source of truth for *where things stand day to day*.

This `docs/` tree is for the things that don't belong in either: point-in-time audits, tracked issues, and forward plans.

```
docs/
├── audits/     Point-in-time reviews of the project (correctness, security, design honesty)
├── issues/     Known problems found by audits — severity, root cause, fix plan, status
├── plans/      Forward-looking roadmaps and prioritization
└── changelog.md  Chronological record of notable changes (what shipped, when, why)
```

## Index

| Doc | What it is |
|---|---|
| [audits/2026-06-15-progress-audit.md](audits/2026-06-15-progress-audit.md) | Brutally-honest progress audit: what's good, what's questionable, security, and the test/CI gap |
| [issues/alert-loss-trio.md](issues/alert-loss-trio.md) | Three ways an alert can be silently lost (H1/H2/H3) — the alert-delivery reliability gaps |
| [issues/audit-findings.md](issues/audit-findings.md) | The rest of the audit findings (auth gap, CORS, Telegram error handling, frontend) with status |
| [plans/roadmap.md](plans/roadmap.md) | Prioritized next steps from the audit's perspective |
| [changelog.md](changelog.md) | What has been changed and verified |

## Conventions

- Each issue has a stable id (`H1`, `SEC-1`, …) so it can be referenced from commits and the changelog.
- Status values: `open` → `in progress` → `fixed` (with the commit) or `accepted` (deliberately not fixing, with reasoning).
- Audits are dated and immutable — a later review gets a new file; we don't rewrite history.
