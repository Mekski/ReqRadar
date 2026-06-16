# ReqRadar

Watchlist-first hiring intelligence ("radar for job reqs") for **30 target companies**: posting detection with 3 years of backfilled timing history, "expected open" SWE-intern seasonality, posted-pay-range extraction, **on-demand LLM features (fit-score: resume↔JD; grounded-search company sentiment)**, sub-minute detection-to-alert via Telegram (watchlist + firehose). Single user (Mark), resume project for Summer 2027 SWE internship recruiting.

**Read DESIGN.md before making any architectural change.** It is the locked design; §10 (rejected alternatives) lists decisions that must not be silently re-introduced.

**Working docs live in [`docs/`](docs/)** — point-in-time audits, tracked issues (with stable ids like H1/SEC-1), forward plans, and a changelog. Start at [`docs/README.md`](docs/README.md). Keep it updated as you work: log notable changes in `docs/changelog.md` and flip issue statuses when you fix them.

## Where things stand & where to pick up (2026-06-16)

**Working end-to-end, CI green, on `main`:** the full collector → NATS → processor → Postgres pipeline; watchlist + firehose Telegram alerts + the REST API; a Next.js dashboard. Telegram verified (bot @ReqRadarBot, 607ms detect-to-alert).

**Shipped since the 2026-06-15 notes below (all live + verified):** **3 collectors** (simplify-listings + git-history backfill, greenhouse, ashby); **30 watchlist companies**; **pay extraction** from ATS JDs; **"expected open"** (data-derived + curated `expected_estimate` for every company); a full **`make backfill`** (timing 2023-08→present); the **firehose fixed** (backfill no longer pollutes it; real posting dates). **Two LLM features (free-tier Gemini), live with Mark's key:** **fit score** (resume PDF↔JD, rubric-calibrated, cached forever) and the **grounded-search sentiment card** (on-demand per company). See the LLM roadmap section + changelog for details.

**Mark's status (2026-06-16):** happy with the project as a portfolio piece — useful + applicable. **Not done** (more features planned), but at a comfortable pause-point; starting to write resume bullets. **The one headline remaining task he wants: DEPLOYMENT** — make it run 24/7 so Telegram alerts (watchlist + firehose) arrive even when his laptop is off. His laptop can't do this (must be awake); needs a free always-on host. Recommended path = **Oracle Cloud Always Free VM + Docker Compose** (matches DESIGN's "single VM, Docker Compose, no k8s"). What's missing: **Dockerfiles for the 3 Go services + a prod docker-compose (services + Postgres + NATS + restart policies + auto migrate/seed) + a deploy guide.** Mark won't pay (free tier only); see `prefers-free-tooling` memory. The collector is a **persistent daemon** (internal tickers), not a cron job — always-on = keeping the process supervised, which is what the deploy provides.

**Dashboard visual identity (2026-06-15):** re-skinned to **minimalist dark-blue, aligned to Mark's portfolio** (markmairs.vercel.app, source at `/Users/markmairs/CS/portfolio`) — navy `#0d1421` bg, single bright-blue accent `#4f8cff`, blue-tinted thin borders, **JetBrains Mono** as display/label font + **Geist** body, subtle fade/rise + glow-on-hover, generous whitespace. Home groups companies by **S/A/B/C tier** (replaced top/mid/low; seed values updated, re-seed needed). Logos via **Google favicon** (`google.com/s2/favicons` — Clearbit's API is dead). Card stats are now **"roles" + "last active"** (dropped confusing "tracked"/"signals"). **Mark's taste = minimalist, NOT flashy — do not add loud colors or ambitious themes.** An earlier amber "mission-control" version was scrapped for being too generic/loud. UX iteration is ongoing — he's reviewing the home page; other pages may still get feedback. Don't treat the dashboard as final.

**Pick up here — ACTIVE THREAD is dashboard UX (Mark's feedback 2026-06-15):**

1. **Reframe the company detail page around the flagship question "when will SWE intern apps drop this season?"** Today its "posting activity" chart is raw monthly history — not useful, and usually empty (companies aren't backfilled). Instead: aggregate historical `posting_opened` by **month-of-year** (across years) and show an **expected-open window** ("historically opens Aug–Oct, peak Sept"). Handle empty state. Also: **open/closed is ambiguous** (needs `posting_closed`, #4) and **"recent updates" all say "new"** (badge labels event type, not recency — show real dates + what changed, de-jargon).
1a. **"Expected open" — ✅ SHIPPED (2026-06-15).** Cards show expected-open = peak month of **summer + SWE** posting_opened (`is_summer` + category filters), with a **curated-seed fallback** for sparse companies.
- **Confidence gate** (`attachExpected` in `internal/store/api.go`, `minSamples=5`): a data-derived month shows only when a company has ≥5 summer-SWE postings. 9/20 qualify on real data (Amazon→Dec, Apple→May, EA→Oct, Epic→Oct, Meta→Sep, Microsoft→Oct, NVIDIA→Sep, Tesla→Jun, TikTok→Aug).
- **Fallback = CURATED SEED, NOT a runtime LLM** (Mark's call). The other 11 are seeded as `expected_estimate` in `seed/watchlist.yaml` (stored in `entities.metadata`, served on `/api/companies`, rendered in `CompanyCard.tsx`). Research basis (deep-research workflow + targeted web search, 2026-06-15): **month estimates** Google→Oct, Riot→Sep, Databricks→Aug, GitHub→Sep, LinkedIn→Sep, Roblox→Jul (labeled "≈ est."); **"rolling"** for Anthropic/OpenAI/xAI/Notion/Niantic (hire continuously, no documented season — shown literally as "rolling", no est. tag). UI priority: data-month → estimate → "rolling" → "—". Each estimate carries a source citation as a YAML comment. **Lowest-confidence: Roblox→Jul, LinkedIn→Sep** (revisit if data contradicts). Do NOT build an LLM call for this.
- **Seed drift reconciled (2026-06-15):** `seed/watchlist.yaml` now mirrors the live watchlist (tiers reconciled to dashboard edits; Tesla/TikTok/Databricks/GitHub/LinkedIn added) so `make seed` is reproducible + non-destructive. **`make seed` does NOT auto-load `.env`** — source it (`set -a && . ./.env && set +a && go run ./cmd/seed`) or it overwrites the user's `telegram_chat_id` with a placeholder. Orphan entities `Capital One`/`Stripe` exist off-watchlist (earlier dashboard experiments) — harmless, not seeded.
- **Future genuine LLM (sentiment summaries / fuzzy entity disambiguation, DESIGN §3.4): use a FREE tier** — Gemini Flash (1,500 req/day, no card), Groq, Cerebras (1M tok/day), Mistral (1B tok/mo). Mark won't pay; **no Anthropic paid API** (`ANTHROPIC_API_KEY` removed from `.env.example` 2026-06-15 — no LLM client code exists). See `prefers-free-tooling` memory.

1b. **Watchlist card redesign (Mark, 2026-06-15):** the per-card timing sparkline reads as ambiguous — replace it with **"Expected: <month>"** (left, = SWE seasonality peak per company; add an `expected_open` field to `/api/companies` via a SWE month-of-year query) + a real metric on the right. **Pay range NOW WIRED via the Greenhouse collector** (2026-06-15, see #5): card right-side shows the company's SWE-intern posted pay ("$72/hr" etc.), "—" when none is known yet (SimplifyJobs still has no salary field, so pay only comes from Greenhouse/Ashby JD text; off-season there are ~0 open SWE interns so it reads "—" for now). Earlier this was a "???" placeholder. Also note: card timing was a render-bug source — keep bars as direct children of a fixed-height row.

2. **NEW PAGE: Calendar** (Mark's idea — the flagship feature done right). Nav becomes **watchlist · calendar · firehose**. A month calendar with watchlist **company logos placed on the months they historically open SWE intern apps** (expect heavy Aug–Sept clustering) — see ALL companies' timing at once instead of clicking each. Derive from per-company `posting_opened` month-of-year aggregation; add a `/api/calendar` endpoint. Day-level detail TBD.
3. ✅ **`make backfill` DONE (2026-06-15)** — re-ran after the watchlist grew to 30; 34,278 postings across 18 git snapshots → timing now spans **2023-08 → present** (TikTok 636 opens, Microsoft 240, NVIDIA 228, Meta 205, …; new adds lit up too — Pinterest 40, Spotify 22, Stripe 18). 0 alerts (freshness gate held). Data-derived expected-open grew 9 → **11/30** (added Pinterest, Ramp). **7 new adds still blank** (Airbnb, Coinbase, Discord, Figma, Snap, Spotify, Stripe) — sparse summer-SWE history, candidates for a curated `expected_estimate` pass like the original 11. Note: `make backfill` needs `GITHUB_TOKEN` — not in `.env`, but `export GITHUB_TOKEN=$(gh auth token)` works. Re-run after adding companies (the resolver picks them up so their history populates).
4. **`posting_closed` detection** — so "open roles" means currently open (everything's "open" now; see gotchas). Underpins the detail-page open/closed clarity.
5. **More collectors:** ✅ **greenhouse SHIPPED (2026-06-15)** — `internal/collector/greenhouse` polls `boards-api.greenhouse.io/v1/boards/<org>/jobs?content=true` for all orgs in the source config (roblox/anthropic/xai/riotgames/epicgames), filters to internships, and emits one signal per intern req; `normalizeGreenhouse` in the processor derives terms/category from the title (collector stays a volume guard, not a parser). Verified live: 2 intern postings resolved to watchlist, 0 full-time leaked, no alert flood. **Intern filter = word-boundary `\bintern(ship)?s?\b` on title OR department** — substring matching wrongly catches "Internal"/"International" (real false positives). Greenhouse has NO Backfiller (API only exposes open reqs) → live detection + pay only, no history. ✅ **ashby SHIPPED (2026-06-15)** — `internal/collector/ashby` polls `api.ashbyhq.com/posting-api/job-board/<org>?includeCompensation=true` (openai/notion). Intern filter is **exact on the structured `employmentType == "Intern"`** (no title heuristic needed — and OpenAI correctly yields 0 interns, since its "intern" titles were all "Internal"/"International"). `normalizeAshby` resolves company via the **org slug** (Ashby exposes no company name), reusing `inferCategory`/`termsFromTitle`/`extractPay`. Verified live: **Notion's "Software Engineer Intern (Fall 2026)" → category Software Engineering, $57/hr** — the first real pay value on a card. `isListed`-gated (active only); no Backfiller. **Still to add: hn.**
- **✅ Pay extraction SHIPPED (2026-06-15):** `internal/processor/pay.go` `extractPay` parses the JD `content` HTML (`pay_input_ranges` is unused by these orgs). It's deliberately conservative — only extracts when a period keyword (hourly/annual/monthly) sits within ~40 chars of a `$` amount AND the value passes a per-period plausibility band (rejects "$5 billion … annual revenue"). The keyword can be on EITHER side: Greenhouse writes "Hourly Pay Range $72 — $72 USD" (keyword before). Stored in `postings.pay_min/pay_max` (widened to NUMERIC in migration `000008`) + new `pay_period` (`pay_currency` already existed from 000003). Card pay = the company's most recent **SWE-category** intern with pay (`attachPay`, Mark's "standard SWE intern" call), rendered "$72/hr" / "$45–55/hr" / "$120–150k/yr". **Off-season caveat:** Greenhouse has no backfill + it's June, so there are ~0 open SWE interns → cards show "—" for pay until fall. `attachPay` is NOT open/closed-filtered, so once a SWE-intern pay is captured it persists (shows the most recent) even after the role closes. **DECISION (2026-06-15, Mark): real posted pay ONLY — do NOT add a curated pay-estimate fallback** (unlike expected-open). Reasons: the main public intern-pay source (Levels.fyi) is OFF-LIMITS per the legal bright lines, pay varies too much by level/location to seed honestly, and "—" until real data arrives matches Mark's honesty bar. The blank off-season state is intended, not a bug. Verified the pipeline live anyway: Roblox's PhD-intern req extracted `$72/hr` (it's AI/ML, so correctly excluded from the SWE card). **Migration gotcha:** my first 000008 tried to re-ADD pay_min/max (they predate it, from 000003) → atomic rollback left schema_migrations dirty at v8; reset with `UPDATE schema_migrations SET version=7, dirty=false` then re-ran. The shared dev DB means another agent's migration can leave it dirty too — check `SELECT version,dirty FROM schema_migrations`.
6. **Free deployment + CI deploy step** — Oracle Always Free / Fly.io (Mark won't pay; see memory `prefers-free-tooling`).

**Later milestones (Mark deferred, do NOT build yet):** LinkedIn recruiter discovery (registry is person-aware for it, DESIGN §6).

## LLM roadmap (decided 2026-06-16) — FREE TIER ONLY (Gemini/Groq/Cerebras/Mistral; never paid)

Note free-tier rate limits — prioritize, don't ship all at once. Expected-open does NOT use an LLM (curated seed; see item 1a).

**✅ Fit score — SCAFFOLDED (2026-06-16), pending Mark's `GEMINI_API_KEY`.** First LLM feature, built end-to-end except the live call.
- **Provider = Gemini Flash** (free, no credit card; key from aistudio.google.com). `internal/llm` = provider interface + Gemini client (`responseMimeType: application/json`, temp 0.2 for stable scores). `GEMINI_API_KEY`/`GEMINI_MODEL` (default `gemini-2.5-flash`) in config + `.env.example`. **Empty key ⇒ everything works but `POST /api/fit` returns 503 "not configured"** (verified). Mark adds the key + restarts the api → scoring goes live.
- **Resume input = PDF upload** (Mark uses Overleaf, so NO in-app editor). `resumes` table; PDF→text via `github.com/ledongthuc/pdf` (Overleaf/LaTeX PDFs extract cleanly; scanned PDFs error out). Multiple resumes, reusable. `POST/GET/DELETE /api/resumes`.
- **The "fit" tab** (nav is now **watchlist · fit · firehose**): pick a resume → paste a JD **or** pick a watchlist role → score. JD picker is **ATS postings only** (Greenhouse/Ashby carry JD text in the new `postings.jd_text`, populated by the normalizers; SimplifyJobs/firehose have none → paste). Tiered S→C. Result UI: 0–100 + verdict, component-score bars, summary, matched/missing skills, ATS gaps, suggestions.
- **Prompt** (Mark-approved) lives in `internal/fit/prompt.go` — rubric-anchored (Technical 40 / Experience 25 / Impact 15 / Eligibility 10 / ATS 10) so scores are consistent + defensible; intern-calibrated; cites resume evidence; specific suggestions. **v2 (2026-06-16): recalibrated to grade like a selective recruiter** (LLMs over-grade — scores were clustering 80+; now demonstrated>>listed, avg ~45-65, 80+ is earned) + verbose JD-specific suggestions with before→after rewrites. **`promptVersion` const is folded into the cache key** — bump it on ANY prompt change so old cached scores auto-invalidate (the cache is keyed on jd+resume content, not the prompt).
- **Architecture invariants honored:** LLM reached only on-demand from `POST /api/fit`, **never the alert path**; **cache forever** — `fit_scores` UNIQUE (jd_hash, resume_hash) ⇒ one Gemini call per unique (JD, resume) pair, ever (mirrors the resolution cache). Migration `000011`.
- **Gotcha:** `jd_text` (like pay) is set on posting INSERT only, so existing ATS postings need a delete-+-re-poll (or a future re-extract) to populate it — done once for the current set.
- **Gemini JSON gotchas (fixed 2026-06-16, live):** (1) **2.5 Flash spends "thinking" tokens from the output budget** → on longer inputs the JSON truncated mid-structure ("invalid character ',' after object key"). Fix: `thinkingConfig.thinkingBudget=0` + `maxOutputTokens=8192` in `internal/llm/gemini.go`. (2) **`responseSchema` is a trap here** — with the nested fit shape Gemini returned ONLY the top scalar fields (`overall_score`/`verdict`) and dropped the rest; we **do not** use responseSchema — `responseMimeType:application/json` + the prompt's explicit shape + a fence-strip + one retry on malformed JSON is robust and returns the full result. Verified live (score 95, all component scores + skills + suggestions).
- **Next (Phase 2, cheap):** resume/JD optimization tips (same inputs, extend the call).

**✅ Sentiment card — SCAFFOLDED (2026-06-16), pending the same `GEMINI_API_KEY`.** On-demand "what does the community say" report on the **company detail page**.
- **Architecture pivot (Mark's idea, my agreement): grounded web search, NOT HN/Reddit collectors.** Gemini "Grounding with Google Search" (free on 2.5 Flash: **1,500 grounded req/day**, far beyond single-user need) searches the whole public web — Reddit/HN/blogs + Glassdoor/Blind *snippets that surface in results* (we never scrape them ourselves → legally clean). This beat building HN+Reddit collectors: less plumbing, broader/fresher coverage, real citations, and it directly answers Mark's wishlist (OA difficulty, # rounds, intern pay/**housing stipend**, prestige, culture). The deterministic collector pipeline stays for structured/real-time data (postings/timing/alerts); grounded search for the fuzzy/qualitative data — a defensible right-tool-per-job split.
- **On-demand only** (button on the company view), **never auto** — so we never spend a grounded call on a company Mark doesn't care about. **One row per company** (`company_sentiment`, UNIQUE entity_id, migration `000012`); regenerate UPSERTs (replaces the old — "store latest, delete old"). LLM reached only here, never the alert path.
- **Anti-hallucination is built into the prompt** (`internal/sentiment/prompt.go`): answer ONLY from search results; write "_Not enough public information found._" for anything (e.g. housing stipends) it can't source — no guessing. **Citations are the real grounding URIs** from `groundingMetadata` (not model-authored, so unfakeable), stored + shown.
- **Pieces:** `llm.GenerateGrounded` (adds the `google_search` tool, extracts text + source URIs); `internal/sentiment`; `GET/POST /api/companies/{id}/sentiment`; `web/.../SentimentCard.tsx` (react-markdown render + generate/regenerate button + sources + generated-at). Structured markdown (Overall / Prestige / Culture / Interview process / Intern pay & housing / Return offers / Watch-outs / Confidence). Verified graceful-not-configured.

**Wanted (Mark's picks):**
- **Sentiment summaries** — ✅ scaffolded as the grounded **sentiment card** (see above). Interview-prep is now largely folded into its "Interview process" section.
- **Fit score** — ✅ scaffolded (see above).
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
- **Backfill must NOT feed the firehose** (fixed 2026-06-15): `RawSignal.Backfill` flags history-replay signals; the processor stores their watchlist timing but skips `maybeFirehose` for them. Before this, a `make backfill` dumped ~16k historical (404) non-watchlist postings into `firehose_seen`, and since the page sorted by `first_seen` (= record time) they showed as "new". `firehose_seen` now also stores `event_time` (the job's real date_posted, migration `000010`); `RecentFirehose` sorts by it. **Caveat:** old backfill signals already in the NATS SIGNALS stream lack the flag, so a manual JetStream **replay** within the 35-day retention would re-pollute — they age out, and they're not in `raw_signals` (watchlist-only), so a Postgres replay is clean. If the firehose ever refills with old 404s: `TRUNCATE firehose_seen` then `make firehose-prime` (safe once the processor is drained, `num_pending=0`).
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
