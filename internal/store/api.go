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
// (not previously seen). The insert-or-ignore makes "is this new?" a single
// atomic op.
func (s *Store) MarkFirehoseSeen(ctx context.Context, source, externalID, company, title, url, category string) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`INSERT INTO firehose_seen (source, external_id, company, title, url, category)
		 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (source, external_id) DO NOTHING`,
		source, externalID, company, title, url, category)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
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
	Name     string
	Domain   string
	Priority string
	Source   string   // 'seed' | 'manual'
	Aliases  []string // raw; normalized + canonical-name-added inside UpsertCompany
}

// UpsertCompany creates or updates a company entity, its aliases, and the
// watchlist row for userID — the one definition shared by cmd/seed and the
// POST /api/companies handler. Runs inside the caller's transaction (q).
func (s *Store) UpsertCompany(ctx context.Context, q DBTX, userID int64, in CompanyInput) (int64, error) {
	meta, _ := json.Marshal(map[string]any{"priority": in.Priority})
	var entityID int64
	if err := q.QueryRow(ctx,
		`INSERT INTO entities (kind, canonical_name, domain, metadata)
		 VALUES ('company', $1, $2, $3)
		 ON CONFLICT (kind, canonical_name)
		 DO UPDATE SET domain = EXCLUDED.domain, metadata = EXCLUDED.metadata
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

type FirehosePosting struct {
	Company   string    `json:"company"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Category  string    `json:"category"`
	FirstSeen time.Time `json:"first_seen"`
}

// RecentFirehose returns the most recently seen non-watchlist postings.
func (s *Store) RecentFirehose(ctx context.Context, limit int) ([]FirehosePosting, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT company, title, COALESCE(url, ''), COALESCE(category, ''), first_seen
		 FROM firehose_seen ORDER BY first_seen DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FirehosePosting
	for rows.Next() {
		var f FirehosePosting
		if err := rows.Scan(&f.Company, &f.Title, &f.URL, &f.Category, &f.FirstSeen); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ---- Dashboard reads ----

type CompanySummary struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	Domain       string         `json:"domain"`
	Priority     string         `json:"priority"`
	OpenPostings int            `json:"open_postings"`
	TotalEvents  int            `json:"total_events"`
	Timing       []TimingBucket `json:"timing"` // last 12 months, for the card sparkline
}

func (s *Store) WatchlistCompanies(ctx context.Context, userID int64) ([]CompanySummary, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT e.id, e.canonical_name, COALESCE(e.domain, ''), COALESCE(e.metadata->>'priority', ''),
		        (SELECT count(*) FROM postings p WHERE p.entity_id = e.id AND p.status = 'open'),
		        (SELECT count(*) FROM events ev WHERE ev.entity_id = e.id)
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
		if err := rows.Scan(&c.ID, &c.Name, &c.Domain, &c.Priority, &c.OpenPostings, &c.TotalEvents); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachTiming(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachTiming fills each company's last-12-months posting_opened histogram in
// one grouped query (avoids an N+1 of per-company /timing calls).
func (s *Store) attachTiming(ctx context.Context, companies []CompanySummary) error {
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
		`SELECT entity_id, to_char(event_time, 'YYYY-MM') AS month, count(*)
		 FROM events
		 WHERE type = 'posting_opened'
		   AND event_time >= date_trunc('month', now()) - interval '11 months'
		   AND entity_id = ANY($1)
		 GROUP BY entity_id, month
		 ORDER BY entity_id, month`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var entityID int64
		var b TimingBucket
		if err := rows.Scan(&entityID, &b.Month, &b.Count); err != nil {
			return err
		}
		if c := idx[entityID]; c != nil {
			c.Timing = append(c.Timing, b)
		}
	}
	return rows.Err()
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

type TimingBucket struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// CompanyTiming returns the monthly posting-open histogram — the flagship
// "when do they historically open apps" feature.
func (s *Store) CompanyTiming(ctx context.Context, entityID int64) ([]TimingBucket, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT to_char(event_time, 'YYYY-MM') AS month, count(*)
		 FROM events WHERE entity_id = $1 AND type = 'posting_opened'
		 GROUP BY month ORDER BY month`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimingBucket
	for rows.Next() {
		var t TimingBucket
		if err := rows.Scan(&t.Month, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type OpenPosting struct {
	ID        int64     `json:"id"`
	Company   string    `json:"company"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Locations []string  `json:"locations"`
	FirstSeen time.Time `json:"first_seen"`
}

func (s *Store) OpenPostings(ctx context.Context, userID int64, limit int) ([]OpenPosting, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT p.id, e.canonical_name, p.title, COALESCE(p.url, ''), p.locations, p.first_seen
		 FROM postings p
		 JOIN entities e ON e.id = p.entity_id
		 JOIN watchlist w ON w.entity_id = p.entity_id AND w.user_id = $1
		 WHERE p.status = 'open'
		 ORDER BY p.first_seen DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OpenPosting
	for rows.Next() {
		var p OpenPosting
		if err := rows.Scan(&p.ID, &p.Company, &p.Title, &p.URL, &p.Locations, &p.FirstSeen); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
