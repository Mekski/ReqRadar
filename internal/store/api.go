package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Mekski/reqradar/internal/entity"
)

// ---- Alert dispatcher ----

type Watcher struct {
	UserID      int64
	ChatID      string
	AlertConfig json.RawMessage
}

// UsersWatchingEntity returns the users who have this entity on their watchlist.
func (s *Store) UsersWatchingEntity(ctx context.Context, entityID int64) ([]Watcher, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT u.id, u.telegram_chat_id, w.alert_config
		 FROM watchlist w JOIN users u ON u.id = w.user_id
		 WHERE w.entity_id = $1`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Watcher
	for rows.Next() {
		var w Watcher
		if err := rows.Scan(&w.UserID, &w.ChatID, &w.AlertConfig); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) InsertAlert(ctx context.Context, userID, eventID int64, detectToAlertMS int) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO alerts (user_id, event_id, channel, sent_at, detect_to_alert_ms)
		 VALUES ($1, $2, 'telegram', now(), $3)`, userID, eventID, detectToAlertMS)
	return err
}

// AllUserChatIDs returns every user's Telegram chat id (firehose alerts are not
// watchlist-scoped, so they go to all users).
func (s *Store) AllUserChatIDs(ctx context.Context) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT telegram_chat_id FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// MarkFirehoseSeen records a firehose posting and returns true if it was new
// (not previously seen) — the non-transactional variant for callers (e.g.
// firehose-prime) that aren't inside a tx. It just runs MarkFirehoseSeenTx against
// the pool so the SQL lives in one place.
func (s *Store) MarkFirehoseSeen(ctx context.Context, source, externalID, company, title, url, category string, eventTime time.Time) (bool, error) {
	return s.MarkFirehoseSeenTx(ctx, s.Pool, source, externalID, company, title, url, category, eventTime)
}

// FirstUserID returns the single v1 user's id.
func (s *Store) FirstUserID(ctx context.Context) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&id)
	return id, err
}

// ---- Watchlist editing (seed + dashboard share this) ----

// CompanyInput is the data needed to add/update a watchlist company.
type CompanyInput struct {
	Name             string
	Domain           string
	Priority         string
	Source           string   // 'seed' | 'manual'
	Aliases          []string // raw; normalized + canonical-name-added inside UpsertCompany
	ExpectedEstimate string   // curated summer-SWE open month, e.g. "Sep" (seed only; blank for manual adds)
}

// UpsertCompany creates or updates a company entity, its aliases, and the
// watchlist row for userID — the one definition shared by cmd/seed and the
// POST /api/companies handler. Runs inside the caller's transaction (q).
func (s *Store) UpsertCompany(ctx context.Context, q DBTX, userID int64, in CompanyInput) (int64, error) {
	// Only include keys we actually have, so a manual add with a blank field can't
	// null out an existing value. On conflict we MERGE (metadata || EXCLUDED.metadata)
	// rather than replace, so this writer only touches the keys it sets and leaves
	// other UI-managed keys intact. Precedence: for a key present on both sides the
	// incoming value wins — so `make seed` stays authoritative for priority /
	// expected_estimate (the YAML is the reconciled source of truth), while a UI-only
	// key the seed doesn't carry survives a re-seed. (Edit tiers in the YAML if you
	// re-seed; an interim UI tier edit persists until the next seed.)
	metaMap := map[string]any{}
	if in.Priority != "" {
		metaMap["priority"] = in.Priority
	}
	if in.ExpectedEstimate != "" {
		metaMap["expected_estimate"] = in.ExpectedEstimate
	}
	meta, _ := json.Marshal(metaMap)
	var entityID int64
	if err := q.QueryRow(ctx,
		`INSERT INTO entities (kind, canonical_name, domain, metadata)
		 VALUES ('company', $1, $2, $3)
		 ON CONFLICT (kind, canonical_name)
		 DO UPDATE SET domain = EXCLUDED.domain,
		               metadata = COALESCE(entities.metadata, '{}') || EXCLUDED.metadata
		 RETURNING id`,
		in.Name, in.Domain, meta).Scan(&entityID); err != nil {
		return 0, err
	}

	source := in.Source
	if source == "" {
		source = "manual"
	}
	seen := map[string]bool{}
	for _, a := range append([]string{in.Name}, in.Aliases...) {
		n := entity.Normalize(a)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		if _, err := q.Exec(ctx,
			`INSERT INTO entity_aliases (entity_id, alias, source, confidence)
			 VALUES ($1, $2, $3, 1.0)
			 ON CONFLICT (alias) DO UPDATE SET entity_id = EXCLUDED.entity_id, source = EXCLUDED.source, confidence = 1.0`,
			entityID, n, source); err != nil {
			return 0, err
		}
	}

	if _, err := q.Exec(ctx,
		`INSERT INTO watchlist (user_id, entity_id, alert_config) VALUES ($1, $2, '{}')
		 ON CONFLICT (user_id, entity_id) DO NOTHING`, userID, entityID); err != nil {
		return 0, err
	}
	return entityID, nil
}

// RemoveWatchlistCompany drops the watchlist row only — entity, events, and
// postings are retained so history survives and re-adding is lossless.
func (s *Store) RemoveWatchlistCompany(ctx context.Context, userID, entityID int64) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM watchlist WHERE user_id = $1 AND entity_id = $2`, userID, entityID)
	return err
}

// UpdateCompanyTier changes a company's tier (the metadata.priority field).
func (s *Store) UpdateCompanyTier(ctx context.Context, entityID int64, tier string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE entities SET metadata = jsonb_set(COALESCE(metadata, '{}'), '{priority}', to_jsonb($2::text))
		 WHERE id = $1 AND kind = 'company'`, entityID, tier)
	return err
}

// CompanyExpectedEstimate returns a company's stored expected-open estimate (the
// curated or previously-researched fallback month), or "" if none. Used to skip a
// redundant grounded search when an estimate already exists.
func (s *Store) CompanyExpectedEstimate(ctx context.Context, entityID int64) (string, error) {
	var est string
	err := s.Pool.QueryRow(ctx,
		`SELECT COALESCE(metadata->>'expected_estimate', '') FROM entities WHERE id = $1`, entityID).Scan(&est)
	return est, err
}

// SetExpectedEstimate records a researched expected-open estimate (month or
// "rolling") with its source. The WHERE guard only writes when no estimate exists
// yet, so it can never overwrite a hand-curated value or a prior result — making
// "research at most once, curated always wins" enforced in SQL.
func (s *Store) SetExpectedEstimate(ctx context.Context, entityID int64, month, source, sourceURL string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE entities
		 SET metadata = COALESCE(metadata, '{}') || jsonb_build_object(
		       'expected_estimate', $2::text,
		       'expected_estimate_source', $3::text,
		       'expected_estimate_url', $4::text)
		 WHERE id = $1 AND kind = 'company'
		   AND COALESCE(metadata->>'expected_estimate', '') = ''`,
		entityID, month, source, sourceURL)
	return err
}

// BlankEstimateCompanies returns watchlist companies (for userID) that have no
// expected-open estimate yet — the backfill targets for the enrich-expected command.
func (s *Store) BlankEstimateCompanies(ctx context.Context, userID int64) ([]struct {
	ID   int64
	Name string
}, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT e.id, e.canonical_name
		 FROM watchlist w JOIN entities e ON e.id = w.entity_id
		 WHERE w.user_id = $1 AND COALESCE(e.metadata->>'expected_estimate', '') = ''
		 ORDER BY e.canonical_name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID   int64
		Name string
	}
	for rows.Next() {
		var r struct {
			ID   int64
			Name string
		}
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type FirehosePosting struct {
	Company   string     `json:"company"`
	Title     string     `json:"title"`
	URL       string     `json:"url"`
	Category  string     `json:"category"`
	EventTime *time.Time `json:"event_time"` // the job's posting date (null for legacy rows)
	FirstSeen time.Time  `json:"first_seen"`
}

// RecentFirehose returns the most recently *posted* non-watchlist internships
// (ordered by the job's own date, not when we recorded it — so a backfill or a
// re-prime can't make stale postings look new). Rows without a known posting date
// sort last.
func (s *Store) RecentFirehose(ctx context.Context, limit int) ([]FirehosePosting, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT company, title, COALESCE(url, ''), COALESCE(category, ''), event_time, first_seen
		 FROM firehose_seen ORDER BY event_time DESC NULLS LAST, first_seen DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FirehosePosting
	for rows.Next() {
		var f FirehosePosting
		if err := rows.Scan(&f.Company, &f.Title, &f.URL, &f.Category, &f.EventTime, &f.FirstSeen); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ---- Dashboard reads ----

type CompanySummary struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Domain       string `json:"domain"`
	Priority     string `json:"priority"`
	ExpectedOpen   string `json:"expected_open"`             // data-derived SWE seasonality peak month, e.g. "Aug" ("" if too few samples)
	ExpectedEst    string `json:"expected_estimate"`         // fallback month when data is sparse (the UI labels it "≈ est.")
	ExpectedEstSrc string `json:"expected_estimate_source"`  // "" | "curated" | "llm" — provenance of the estimate
	ExpectedEstURL string `json:"expected_estimate_url"`     // citation for an "llm"-researched estimate ("" otherwise)

	// Posted pay of the company's most recent SWE-category internship (the card's
	// pay figure). PayPeriod == "" means no SWE-intern pay is known yet.
	PayMin    float64 `json:"pay_min"`
	PayMax    float64 `json:"pay_max"`
	PayPeriod string  `json:"pay_period"`
}

var monthAbbr = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func (s *Store) WatchlistCompanies(ctx context.Context, userID int64) ([]CompanySummary, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT e.id, e.canonical_name, COALESCE(e.domain, ''), COALESCE(e.metadata->>'priority', ''),
		        COALESCE(e.metadata->>'expected_estimate', ''),
		        COALESCE(e.metadata->>'expected_estimate_source', ''),
		        COALESCE(e.metadata->>'expected_estimate_url', '')
		 FROM watchlist w JOIN entities e ON e.id = w.entity_id
		 WHERE w.user_id = $1
		 ORDER BY e.canonical_name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CompanySummary
	for rows.Next() {
		var c CompanySummary
		if err := rows.Scan(&c.ID, &c.Name, &c.Domain, &c.Priority, &c.ExpectedEst, &c.ExpectedEstSrc, &c.ExpectedEstURL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachExpected(ctx, out); err != nil {
		return nil, err
	}
	if err := s.attachPay(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachPay sets each company's card pay = the most recent SWE-category
// internship posting that has an extracted pay range. "Standard SWE intern pay"
// is what Mark wants surfaced; non-SWE intern roles (e.g. a PhD applied-scientist
// req) carry pay but don't represent it, so they're excluded here.
func (s *Store) attachPay(ctx context.Context, companies []CompanySummary) error {
	if len(companies) == 0 {
		return nil
	}
	ids := make([]int64, len(companies))
	idx := make(map[int64]*CompanySummary, len(companies))
	for i := range companies {
		ids[i] = companies[i].ID
		idx[companies[i].ID] = &companies[i]
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT DISTINCT ON (entity_id) entity_id, pay_min, pay_max, pay_period
		 FROM postings
		 WHERE entity_id = ANY($1) AND pay_period IS NOT NULL
		   AND category = ANY(ARRAY['Software','Software Engineering'])
		 ORDER BY entity_id, first_seen DESC`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var entityID int64
		var minV, maxV float64
		var period string
		if err := rows.Scan(&entityID, &minV, &maxV, &period); err != nil {
			return err
		}
		if c := idx[entityID]; c != nil {
			c.PayMin, c.PayMax, c.PayPeriod = minV, maxV, period
		}
	}
	return rows.Err()
}

// attachExpected sets each company's expected-open month = the peak month-of-year
// of its SWE-category posting_opened events (one grouped query).
func (s *Store) attachExpected(ctx context.Context, companies []CompanySummary) error {
	if len(companies) == 0 {
		return nil
	}
	ids := make([]int64, len(companies))
	idx := make(map[int64]*CompanySummary, len(companies))
	for i := range companies {
		ids[i] = companies[i].ID
		idx[companies[i].ID] = &companies[i]
	}
	// ORDER BY count DESC, m ASC makes the peak-month pick deterministic: the first
	// row per entity is the highest-count month, ties broken by earliest month — so
	// a tie no longer flips between page loads (the loop below takes strictly-greater,
	// so the first row wins and equal-count later months don't displace it).
	rows, err := s.Pool.Query(ctx,
		`SELECT e.entity_id, EXTRACT(MONTH FROM e.event_time)::int AS m, count(*) AS c
		 FROM events e JOIN postings p ON p.id = e.posting_id
		 WHERE e.type = 'posting_opened' AND e.entity_id = ANY($1) AND p.is_summer
		   AND p.category = ANY(ARRAY['Software','Software Engineering'])
		 GROUP BY e.entity_id, m
		 ORDER BY e.entity_id, c DESC, m ASC`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	type acc struct{ peakMonth, peakCount, total int }
	stats := map[int64]*acc{}
	for rows.Next() {
		var entityID, month, count int64
		if err := rows.Scan(&entityID, &month, &count); err != nil {
			return err
		}
		a := stats[entityID]
		if a == nil {
			a = &acc{}
			stats[entityID] = a
		}
		a.total += int(count)
		if int(count) > a.peakCount && month >= 1 && month <= 12 {
			a.peakCount = int(count)
			a.peakMonth = int(month)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Only trust the data-derived month with enough summer-SWE history; sparse
	// companies (1–2 postings = noise) are left blank, so the UI falls back to the
	// curated expected_estimate from the seed (entities.metadata).
	const minSamples = 5
	for id, a := range stats {
		if a.total >= minSamples && a.peakMonth >= 1 {
			if c := idx[id]; c != nil {
				c.ExpectedOpen = monthAbbr[a.peakMonth-1]
			}
		}
	}
	return nil
}

type TimelineEvent struct {
	Type      string          `json:"type"`
	EventTime time.Time       `json:"event_time"`
	Data      json.RawMessage `json:"data"`
}

func (s *Store) CompanyTimeline(ctx context.Context, entityID int64, limit int) ([]TimelineEvent, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT type, event_time, data FROM events
		 WHERE entity_id = $1 ORDER BY event_time DESC LIMIT $2`, entityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimelineEvent
	for rows.Next() {
		var e TimelineEvent
		if err := rows.Scan(&e.Type, &e.EventTime, &e.Data); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type SeasonBucket struct {
	Month int `json:"month"` // 1–12
	Count int `json:"count"`
}

// CompanySeasonality aggregates posting_opened events by month-of-year across ALL
// years — the seasonal pattern that answers "when does this company open roles?"
// If categories is non-empty, it restricts to postings in those categories
// (joining events → postings), so the chart can show SWE-intern roles only.
func (s *Store) CompanySeasonality(ctx context.Context, entityID int64, categories []string) ([]SeasonBucket, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT EXTRACT(MONTH FROM e.event_time)::int AS m, count(*)
		 FROM events e
		 JOIN postings p ON p.id = e.posting_id
		 WHERE e.entity_id = $1 AND e.type = 'posting_opened' AND p.is_summer
		   AND ($2::text[] IS NULL OR p.category = ANY($2))
		 GROUP BY m ORDER BY m`, entityID, categories)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeasonBucket
	for rows.Next() {
		var b SeasonBucket
		if err := rows.Scan(&b.Month, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

