# Résumé bullets — DRAFTS (potential, subject to change)

> ⚠️ **These are DRAFTS / candidates, not final.** They will change as features are
> added and as Mark refines wording/emphasis. Nothing here is locked. Every metric is
> meant to be **real and interview-defensible** (no fabricated impact, no user-scale
> claims — it's single-user; the strength is systems + data-engineering depth).
> Drafted 2026-06-16; revisit whenever the project grows.

## Current draft set

**ReqRadar — Real-time hiring-intelligence platform** · Go, NATS JetStream, PostgreSQL, Next.js/TypeScript, Docker, GitHub Actions

- **Engineered an event-driven backend of three Go services** over NATS JetStream (collector → processor → API) that detects new internship postings and delivers **sub-second Telegram alerts** (measured 607 ms detection-to-alert).
- **Built a plugin-based collector framework** with three sources (a GitHub aggregator + Greenhouse/Ashby APIs) and **reconstructed 3 years / 34K+ postings of application-timing history** by mining git snapshots — surfacing *when* each company opens SWE-intern applications.
- **Hardened the alert path against failure** with idempotent, content-hash-deduplicated processing and a **transactional outbox**, so restarts and replays never double-send or drop alerts (validated on 5.6K duplicate signals).
- **Added two on-demand AI features on a free-tier LLM (Gemini):** a rubric-calibrated résumé-to-JD fit score and a **Google-Search-grounded** company-sentiment report, both cached permanently to respect API limits.
- *(optional 5th)* Shipped a Next.js/TypeScript dashboard + a GitHub Actions CI pipeline (lint, unit, and **integration tests against real Postgres + NATS**).

## Defensibility notes (so every line survives an interview)

- Real, citable metrics: **3 services / NATS**, **607 ms** = detect→alert *latency* (NOT "the instant a job is posted" — sources are polled, so always frame it as latency), **3 years / 34,278 postings** backfilled across 18 git snapshots, **5,609 duplicate signals** deduped to 104 postings.
- Deliberately **no fake impact metrics** (no "served N users", no "improved X by Y%") — single-user tool; lead with systems/data depth instead.
- "microservices" is fair (3 services over a message bus); "real-time" = sub-second alerts. Both defensible.

## Open tailoring decisions (discuss before finalizing)

1. **Emphasis** — lean distributed-systems/backend (best for most SWE intern roles) vs. balance with the AI/LLM angle (good for the AI labs on the watchlist).
2. **Length** — most résumés give a project **3 bullets**; trim to the strongest 3, or keep 4–5 if space allows.
3. **Format** — standalone "Projects" entry (assumed here) vs. STAR-style vs. one-liners.

## Likely future additions that could become bullets

- Free always-on **deployment** (Oracle Always Free VM + Docker Compose) + CI/CD deploy step → a "containerized & deployed a multi-service system to a free cloud VM" bullet.
- `posting_closed` detection, the Calendar view, HN collector / more LLM features.
