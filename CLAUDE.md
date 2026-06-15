# ReqRadar

Watchlist-first hiring intelligence ("radar for job reqs") for ~15 target companies: posting detection with 3 years of backfilled timing history, JD version diffing, posted-pay-range extraction, HN/Reddit sentiment summaries, sub-minute detection-to-alert via Telegram. Single user (Mark), resume project for Summer 2027 SWE internship recruiting.

**Read DESIGN.md before making any architectural change.** It is the locked design; §10 (rejected alternatives) lists decisions that must not be silently re-introduced.

**Working docs live in [`docs/`](docs/)** — point-in-time audits, tracked issues (with stable ids like H1/SEC-1), forward plans, and a changelog. Start at [`docs/README.md`](docs/README.md). Keep it updated as you work: log notable changes in `docs/changelog.md` and flip issue statuses when you fix them.

## Where things stand & where to pick up (2026-06-15)

**Working end-to-end, CI green, on `main`:** the full collector → NATS → processor → Postgres pipeline (Milestone A); watchlist + firehose Telegram alerts and the REST API (Milestone B backend); and a Next.js dashboard. The dev DB holds ~3 years of backfilled timing for 13/15 companies and a primed firehose (~944 rows). Telegram works (bot @ReqRadarBot, 607ms detect-to-alert verified).

**Dashboard visual identity (2026-06-15):** re-skinned to **minimalist dark-blue, aligned to Mark's portfolio** (markmairs.vercel.app, source at `/Users/markmairs/CS/portfolio`) — navy `#0d1421` bg, single bright-blue accent `#4f8cff`, blue-tinted thin borders, **JetBrains Mono** as display/label font + **Geist** body, subtle fade/rise + glow-on-hover, generous whitespace. Home groups companies by **S/A/B/C tier** (replaced top/mid/low; seed values updated, re-seed needed). Logos via **Google favicon** (`google.com/s2/favicons` — Clearbit's API is dead). Card stats are now **"roles" + "last active"** (dropped confusing "tracked"/"signals"). **Mark's taste = minimalist, NOT flashy — do not add loud colors or ambitious themes.** An earlier amber "mission-control" version was scrapped for being too generic/loud. UX iteration is ongoing — he's reviewing the home page; other pages may still get feedback. Don't treat the dashboard as final.

**Pick up here — ACTIVE THREAD is dashboard UX (Mark's feedback 2026-06-15):**

1. **Reframe the company detail page around the flagship question "when will SWE intern apps drop this season?"** Today its "posting activity" chart is raw monthly history — not useful, and usually empty (companies aren't backfilled). Instead: aggregate historical `posting_opened` by **month-of-year** (across years) and show an **expected-open window** ("historically opens Aug–Oct, peak Sept"). Handle empty state. Also: **open/closed is ambiguous** (needs `posting_closed`, #4) and **"recent updates" all say "new"** (badge labels event type, not recency — show real dates + what changed, de-jargon).
1a. **"Expected open" — ✅ SHIPPED (2026-06-15).** Cards show expected-open = peak month of **summer + SWE** posting_opened (`is_summer` + category filters), with a **curated-seed fallback** for sparse companies.
- **Confidence gate** (`attachExpected` in `internal/store/api.go`, `minSamples=5`): a data-derived month shows only when a company has ≥5 summer-SWE postings. 9/20 qualify on real data (Amazon→Dec, Apple→May, EA→Oct, Epic→Oct, Meta→Sep, Microsoft→Oct, NVIDIA→Sep, Tesla→Jun, TikTok→Aug).
- **Fallback = CURATED SEED, NOT a runtime LLM** (Mark's call). The other 11 are seeded as `expected_estimate` in `seed/watchlist.yaml` (stored in `entities.metadata`, served on `/api/companies`, rendered in `CompanyCard.tsx`). Research basis (deep-research workflow + targeted web search, 2026-06-15): **month estimates** Google→Oct, Riot→Sep, Databricks→Aug, GitHub→Sep, LinkedIn→Sep, Roblox→Jul (labeled "≈ est."); **"rolling"** for Anthropic/OpenAI/xAI/Notion/Niantic (hire continuously, no documented season — shown literally as "rolling", no est. tag). UI priority: data-month → estimate → "rolling" → "—". Each estimate carries a source citation as a YAML comment. **Lowest-confidence: Roblox→Jul, LinkedIn→Sep** (revisit if data contradicts). Do NOT build an LLM call for this.
- **Seed drift reconciled (2026-06-15):** `seed/watchlist.yaml` now mirrors the live watchlist (tiers reconciled to dashboard edits; Tesla/TikTok/Databricks/GitHub/LinkedIn added) so `make seed` is reproducible + non-destructive. **`make seed` does NOT auto-load `.env`** — source it (`set -a && . ./.env && set +a && go run ./cmd/seed`) or it overwrites the user's `telegram_chat_id` with a placeholder. Orphan entities `Capital One`/`Stripe` exist off-watchlist (earlier dashboard experiments) — harmless, not seeded.
- **Future genuine LLM (sentiment summaries / fuzzy entity disambiguation, DESIGN §3.4): use a FREE tier** — Gemini Flash (1,500 req/day, no card), Groq, Cerebras (1M tok/day), Mistral (1B tok/mo). Mark won't pay; **no Anthropic paid API** (`ANTHROPIC_API_KEY` removed from `.env.example` 2026-06-15 — no LLM client code exists). See `prefers-free-tooling` memory.

1b. **Watchlist card redesign (Mark, 2026-06-15):** the per-card timing sparkline reads as ambiguous — replace it with **"Expected: <month>"** (left, = SWE seasonality peak per company; add an `expected_open` field to `/api/companies` via a SWE month-of-year query) + a real metric on the right. **Pay range is NOT available** (SimplifyJobs has no salary field; pay lives in JD text → needs the Greenhouse/Ashby collectors + extraction, #5). So right-side = roles count / "last active" for now; pay range only after those collectors. Also note: card timing was a render-bug source — keep bars as direct children of a fixed-height row.

2. **NEW PAGE: Calendar** (Mark's idea — the flagship feature done right). Nav becomes **watchlist · calendar · firehose**. A month calendar with watchlist **company logos placed on the months they historically open SWE intern apps** (expect heavy Aug–Sept clustering) — see ALL companies' timing at once instead of clicking each. Derive from per-company `posting_opened` month-of-year aggregation; add a `/api/calendar` endpoint. Day-level detail TBD.
3. **Run a full `make backfill`** — the "when" features are meaningless without historical timing; newly-added companies (LinkedIn/Tesla/TikTok/GitHub/etc.) have only current data.
4. **`posting_closed` detection** — so "open roles" means currently open (everything's "open" now; see gotchas). Underpins the detail-page open/closed clarity.
5. **More collectors:** ✅ **greenhouse SHIPPED (2026-06-15)** — `internal/collector/greenhouse` polls `boards-api.greenhouse.io/v1/boards/<org>/jobs?content=true` for all orgs in the source config (roblox/anthropic/xai/riotgames/epicgames), filters to internships, and emits one signal per intern req; `normalizeGreenhouse` in the processor derives terms/category from the title (collector stays a volume guard, not a parser). Verified live: 2 intern postings resolved to watchlist, 0 full-time leaked, no alert flood. **Intern filter = word-boundary `\bintern(ship)?s?\b` on title OR department** — substring matching wrongly catches "Internal"/"International" (real false positives). Greenhouse has NO Backfiller (API only exposes open reqs) → live detection + pay only, no history. **Still to add: ashby** (openai/notion — factory registered, no collector package yet; the runner logs "no collector registered" for it, harmless) **+ hn**. **Pay extraction is the immediate follow-up** (pay lives in the JD `content` HTML — `pay_input_ranges` is unused by these orgs; e.g. Roblox PhD intern shows "$72/hr").
6. **Free deployment + CI deploy step** — Oracle Always Free / Fly.io (Mark won't pay; see memory `prefers-free-tooling`).

**Later milestones (Mark deferred, do NOT build yet):** LinkedIn recruiter discovery (registry is person-aware for it, DESIGN §6).

## LLM roadmap (decided 2026-06-16) — FREE TIER ONLY (Gemini/Groq/Cerebras/Mistral; never paid)

Note free-tier rate limits — prioritize, don't ship all at once. Expected-open does NOT use an LLM (curated seed; see item 1a).

**Wanted (Mark's picks):**
- **Sentiment summaries** — summarize HN/Reddit chatter per company. (Genuine LLM use; needs the HN/Reddit collectors first.)
- **Fit score** — given a posting's JD + Mark's resume, score the match + flag missing keywords. High interest. NOTE: introduces a NEW input — Mark's resume (store its text; upload or a config file). Mark felt this **subsumes JD-diff summaries** (see rejected).
- **Resume / JD optimization tips** — per target role, suggest resume tweaks to match the JD / pass ATS. (Pairs with fit score.)
- **Interview-prep brief per company** — synthesize the interview process from public chatter; similar pipeline to sentiment.

**Maybe:** **Category cleanup** — LLM normalizes the messy aggregator categories ("Software" vs "Software Engineering" vs "AI/ML") into clean tags; would directly sharpen the SWE seasonality filter. Mark unsure.

**Rejected:** Weekly digest (Telegram already covers updates); Natural-language alerts (free-tier capacity); JD-diff summaries (Mark can read the JD; fit score covers the need).

**Fuzzy entity disambiguation — my call: NOT needed yet (slightly unsure, flagged).** Deterministic resolution (exact/alias/domain) already handles the structured aggregator/ATS sources, where company names match seeded aliases (verified: 104 postings resolved correctly). The LLM step only earns its place once **messy free-text sources (HN/Reddit) exist**, where strings like "Riot" need context. So: revisit alongside the HN/Reddit sentiment work, not before. If resolution starts missing watchlist companies on structured data, reconsider sooner.

**Alert recency gate (shipped 2026-06-15):** dispatcher only Telegram-alerts when a role's `date_posted` is < 48h old (`alertFreshness` in `internal/api/dispatcher.go`) — events still STORED, but adding a company / backfilling imports old roles silently (no flood). This is the job-watch "only new rows" behavior Mark wanted.

## Build status & run order (updated 2026-06-13)

**✅ MILESTONE A COMPLETE (tasks 1–8).** Full pipeline verified live, CI green: collector (poll + backfill) → NATS → processor (`internal/processor`: durable consumer over `signals.raw.*`, normalize → resolve alias/domain + watchlist filter → tx-per-signal dedupe → Postgres → emit `events.*`). Idempotent (5,609 dupe signals → 104 postings/events). Backfill (`cmd/backfill`) samples git-history snapshots → **1,119 watchlist events spanning 2023-07 to 2026-06**, 13/15 companies. **Next: Milestone B** — api service + Telegram alert dispatcher (consume `events.*`, fire alerts, instrument `detect_to_alert_ms`) + Next.js dashboard.

Processor deferrals (see DESIGN §3.2): LLM resolution step (deterministic alias/domain covers seeded watchlist companies), `posting_closed` detection, pay extraction. `raw_signals` stores watchlist-resolved signals only (lean replay buffer).

Backfill: `make backfill` (needs `GITHUB_TOKEN`; `gh auth token` works) while the processor runs. Samples ~every 60 days back to Aug 2023; current listings.json holds only the live cycle (~8 mo), so multi-year history comes from past commits.

**Milestone B in progress.** DONE: api service (`internal/api`, `cmd/api`) = REST API (`/healthz`, `/api/companies`, `/api/companies/{id}/timeline`, `/api/companies/{id}/timing`, `/api/postings`) + Telegram alert dispatcher. Dispatcher consumes `events.*` with JetStream **`DeliverNew`** so backfill events never alert (NOT a time filter — a newly-detected posting can have an old `date_posted`). Telegram client in `internal/telegram`; bot @ReqRadarBot. **Verified live: re-detected Anthropic posting alerted in 607ms** (sub-minute claim proven), and the 1,119 backfill events correctly did not alert. `make run-api`. **Two-tier alerting (added 2026-06-14):** Tier 1 = watchlist 15 (rich pipeline + alerts, all categories). Tier 2 = **firehose** = non-watchlist SWE+AI/ML internships get lightweight 🆕 alerts (job-watch parity, since ReqRadar supersedes it). Processor routes unresolved postings through `maybeFirehose` → `firehose_seen` dedup → `events.firehose` → dispatcher sends to all users. `cmd/firehose-prime` arms it without flooding (skips watchlist companies). Firehose categories (`internal/processor/firehose.go`, mirrored in cmd/firehose-prime): Software, Software Engineering, AI/ML/Data, "Data Science, AI & Machine Learning".

**Dashboard DONE (2026-06-15):** `web/` = Next.js 16 + TS + Tailwind 4, multi-page (home cards w/ logos + CSS-bar timing sparklines + add/remove company; company detail w/ timing chart + postings + activity; firehose feed tab). `make run-web` (needs api on :8080; CORS open for dev). Watchlist editing: `POST /api/companies` + `DELETE /api/companies/{id}` reuse `store.UpsertCompany`; processor reloads its resolver every 30s so UI-added companies resolve without a restart. API gained `/api/firehose` + `timing` in `/api/companies`. Logos via Clearbit (`logo.clearbit.com/{domain}`). First-pass visuals only — aesthetics pass is later (can use `frontend-design` skill). Note: card "tracked" count = postings ever seen (posting_closed deferred, so it's not "open now"); the timing chart is the accurate hero.

REMAINING Milestone B: more collectors (greenhouse/ashby/hn — factories exist, just need the collector packages), CI/CD deploy step, prod VM (Caddy serving web + proxying /api). Then dashboard aesthetics pass.

Operational gotchas are consolidated in **Gotchas & operational learnings** below.

Local run order (needs Docker + Go 1.26; secrets in `.env`, gitignored):
1. `make dev-up` — postgres, nats (+JetStream), prometheus, grafana
2. `make migrate` — apply schema (idempotent)
3. `make seed` — load `seed/watchlist.yaml` (reads `TELEGRAM_CHAT_ID` from `.env`)
4. `make run-collector` — polls enabled sources, publishes `signals.raw.<source>` to NATS
5. `make run-processor` — consumes signals → Postgres + `events.*`
6. `make run-api` — REST API + alert dispatcher
7. `make firehose-prime` — run ONCE before arming the firehose (records current backlog silently so ~1,000 active postings don't all alert)

Verify data flow: NATS monitoring `http://localhost:8222/jsz?streams=1` (SIGNALS message count); `collector_runs` table for per-run health (status/signal_count/error). Telegram bot is **@ReqRadarBot**, connectivity test already delivered.

## Gotchas & operational learnings (hard-won — read before debugging)

- **`go run ./cmd/X` in background:** it spawns a child `exe`; killing the `go run` PID may leave the child running. Use `pkill -f exe/X` (or build a binary first). A stray collector once polled for hours this way and dumped ~4× duplicate signals into NATS.
- **Purge NATS for a clean test:** NATS has **no mounted volume**, so `docker compose up -d --force-recreate nats` wipes all streams + durable consumers. Do this when a dirty backlog skews a test.
- **`detect_to_alert_ms`** is measured from the signal's `ObservedAt` → send. Huge values (hours) = stale/redelivered queued signals, NOT a bug. A clean run is ~600ms.
- **Backfill-vs-alert suppression uses JetStream `DeliverNew`**, not an `event_time` filter — a newly-detected posting can carry an old `date_posted`, so a time filter would wrongly suppress real alerts.
- **Current `listings.json` only spans the live cycle (~8 months).** Multi-year history comes from sampling PAST git commits (cycle-boundary snapshots carry older `date_posted`). The backfiller samples ~every 60 days back to Aug 2023.
- **Backfill sets `ObservedAt = now`,** so `postings.first_seen` is NOT meaningful for "recently posted" ordering after a backfill. Order timing by `event_time` (= `date_posted`).
- **`posting_closed` is deferred,** so `postings.status` is always `'open'` → "open postings" counts are inflated (= all postings ever seen). The dashboard labels this "tracked" to stay honest. Implementing closure detection is a real next task.
- **Resolver is in-memory and reloads every 30s** (atomic pointer in `internal/processor/resolve.go`). A company added via the dashboard/seed resolves within ~30s, no restart. Side effects of adding a company: a small burst of `posting_opened` alerts for its currently-open roles, and empty historical timing until `make backfill` is re-run.
- **Firehose must be primed once** (`make firehose-prime`) before arming, or ~1,000 already-open postings all alert at once. Prime skips watchlist companies (so `firehose_seen` stays non-watchlist-only).
- **`next/font/google` needs network at build/dev time** (Chakra Petch + IBM Plex Sans/Mono). The env has network (npm + fonts resolved fine); if ever offline, self-host the woff2 or fall back to system fonts.
- **Company logos** are free via `logo.clearbit.com/{domain}` (we store `domain`); render on a light tile so dark logos read on the dark UI.
- **`GITHUB_TOKEN`** is needed for backfill (commits API rate limit); `gh auth token` supplies one.

## Package layout

- `cmd/<svc>` — thin entrypoints: collector, processor, api, migrate, seed
- `internal/signal` — `RawSignal` envelope (the cross-service contract)
- `internal/collector` — `Collector`/`Backfiller` interfaces, `Runner`, per-source `Factory` registration
- `internal/collector/<name>` — one package per collector (e.g. `simplify`)
- `internal/bus` — NATS JetStream wrapper; provisions SIGNALS (`signals.raw.>`, 35d) + EVENTS (`events.>`, 90d) streams
- `internal/store` — pgxpool + typed queries (no raw SQL in services)
- `internal/entity` — shared `Normalize` (the one definition the resolver reuses)
- `internal/config`, `internal/service` — env config + logging/shutdown bootstrap

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
- `ContentHash` is computed over a **canonicalized** payload — strip volatile markers (e.g. Simplify's 🔥/🎓/🛂/🇺🇸 emoji flags) before hashing, or you get phantom re-alerts. Lesson inherited from the predecessor **job-watch** (github.com/Mekski/job-watch), which ReqRadar supersedes. See DESIGN §1 Lineage.

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

- Adding a source = one collector package implementing `Collector` (+ optional `Backfiller`), one `r.Register("<name>", New)` line in `cmd/collector`, and a `sources` row (seed it via `seed/watchlist.yaml`). The `New` factory receives the source row's JSON config; the DB `enabled` flag decides whether it runs. If it takes more than that, the framework regressed.
- Collectors emit ALL current items each poll (active only) and let the processor dedupe by `ExternalID`+`ContentHash` — collectors stay dumb and replayable. Conditional GET (ETag/SHA) avoids re-emitting unchanged data; conditional-GET state is in-memory, so a restart re-emits once and the processor absorbs it.
- Every collector has golden-file fixture tests from real captured payloads. Format drift must fail CI, not silently drop data.
- `log/slog` JSON logging; thread one signal ID through all services via NATS header. Prometheus metrics per service; Grafana dashboard is a demo artifact — keep it presentable.
- Migrations: `golang-migrate`, forward-only.
- Non-goals v1 (do not build): multi-user auth flows, browse/search UX, recruiter enrichment, X collector, Levels-style comp, k8s.
- Application tracker (`applications` table, Telegram "Mark as Applied" callback, dashboard funnel) is a **planned Milestone-C extra**, not v1 core — closes the alert→apply loop as a personal single-user funnel, NOT a generic tracker. Schema seam reserved in DESIGN §4; see §9 item 22.

## Owner context

Mark reads every diff and must be able to defend every line and decision in interviews — prefer clear, idiomatic code over clever code, and when making a non-obvious choice, note the why in the PR/commit, not in code comments. Interview defensibility is a primary design requirement; "simplest thing that works, with the upgrade path documented" is the house style.
