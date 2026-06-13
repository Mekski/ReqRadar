# Watchlist — verified source mapping

*Output of Milestone A, task 1. Verified by probing live public job APIs on 2026-06-13 (not eyeballed). This is the seed data for the entity registry (task 4) and the `sources` config.*

## ATS platform per company

Probed `boards-api.greenhouse.io`, `api.ashbyhq.com`, `api.lever.co` directly; job counts confirm a live board (not just a 200).

| Company | Priority | Platform | Slug | Direct API? | Verified |
|---|---|---|---|---|---|
| Roblox | top | Greenhouse | `roblox` | ✅ | 228 jobs (incl. a 2026 PhD intern posting) |
| Anthropic | top | Greenhouse | `anthropic` | ✅ | 382 jobs |
| OpenAI | top | Ashby | `openai` | ✅ | 731 jobs |
| xAI | top | Greenhouse | `xai` | ✅ | 221 jobs |
| Riot Games | top | Greenhouse | `riotgames` | ✅ | 167 jobs |
| Epic Games | mid | Greenhouse | `epicgames` | ✅ | 136 jobs |
| Notion | mid | Ashby | `notion` | ✅ | 149 jobs |
| Google | mid | Proprietary | — | ❌ aggregator-only | known proprietary |
| Apple | mid | Proprietary | — | ❌ aggregator-only | known proprietary |
| Microsoft | mid | Proprietary | — | ❌ aggregator-only | known proprietary |
| Meta | mid | Proprietary | — | ❌ aggregator-only | known proprietary |
| Amazon | low | Proprietary | — | ❌ aggregator-only | known proprietary |
| NVIDIA | mid | Workday (proprietary) | — | ❌ aggregator-only | no GH/Lever/Ashby board |
| EA | mid | Proprietary (own domain) | — | ❌ aggregator-only | no GH/Lever/Ashby board |
| Niantic | low | ⚠️ in flux | — | ❌ | see note |

**⚠️ Niantic:** `nianticlabs.com/careers` now redirects to **careers.scopely.com** — Scopely (Savvy Games Group) acquired Niantic's games division in 2025. The Ashby slug `niantic` returns an empty board. Niantic as a games employer is effectively folding into Scopely. **Recommendation:** either drop Niantic from the watchlist or repoint it at Scopely. Low priority either way — decide later, not a v1 blocker.

## What this means for the build

- **Two collectors cover 7 of 15 companies — including 6 of your top targets** (Roblox, Anthropic, OpenAI, xAI, Riot, Epic): `greenhouse` (5 companies) + `ashby` (2 companies). This validates the design's core bet — the highest-value sources are clean public JSON APIs.
- **Lever is not needed for v1.** Zero watchlist companies use it. The collector framework still supports adding it in one file the day a future watchlist company needs it, but it's dropped from the v1 build roster.
- **The 8 proprietary companies** (Google, Apple, Microsoft, Meta, Amazon, NVIDIA, EA, Niantic) are detected via the **SimplifyJobs aggregator backfill**, exactly as designed — no bespoke scrapers needed.

## Reddit Data API — ToS verdict: VIABLE for v1, with 3 hard constraints

Read the actual Data API Terms (eff. 2023-06-19). Free tier allows non-commercial personal use (~100 queries/min per OAuth client). Usable, **but**:

1. **No training, ever.** §2.4 and §3.2 forbid using User Content "to train a machine learning or AI model" without rightsholder permission. Our use is **inference-only** (Claude summarizes/classifies sentiment at read time — we never fine-tune on Reddit data), which stays on the right side of this line. Document that distinction; never fine-tune on Reddit content.
2. **Reddit-derived data must be deletable.** §6: on API termination we must "delete any cached or stored User Content... **including any data or models derived from** User Content." This breaks the "events forever, never deleted" model for Reddit specifically. **Schema consequence:** tag content provenance and segregate Reddit-derived rows (raw signals *and* sentiment summaries) into a partition we can drop on demand. Reddit data cannot live in the forever-retained bucket.
3. **Compliance hygiene.** OAuth required, honest User-Agent (no masking, §2.8), real contact info on registration (§1.3), a privacy-policy page (§2.6), non-commercial only (§3.2).

**Recommendation:** because Reddit carries the deletion-on-termination baggage and **Hacker News (Firebase API) has none of it**, keep HN as the primary sentiment source and treat Reddit as an optional Milestone C add. If/when built, its data goes in the segregated deletable partition. Honestly defensible to defer Reddit to v2 entirely to keep v1's persistence story clean — your call when we get there.
