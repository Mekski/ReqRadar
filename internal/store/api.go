package store

import (
	"context"
	"encoding/json"
	"time"
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

// FirstUserID returns the single v1 user's id.
func (s *Store) FirstUserID(ctx context.Context) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&id)
	return id, err
}

// ---- Dashboard reads ----

type CompanySummary struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Domain       string `json:"domain"`
	Priority     string `json:"priority"`
	OpenPostings int    `json:"open_postings"`
	TotalEvents  int    `json:"total_events"`
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
	return out, rows.Err()
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
