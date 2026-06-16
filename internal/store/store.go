// Package store is the Postgres access layer shared by the services. It owns the
// connection pool and the queries; callers get typed methods, not raw SQL. Write
// methods take a DBTX so callers can run them inside a transaction or directly
// against the pool.
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Mekski/reqradar/internal/signal"
)

// DBTX is satisfied by both *pgxpool.Pool and pgx.Tx, so store methods compose
// with transactions.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

// ---- Sources & collector runs (collector service) ----

type SourceConfig struct {
	ID     int64
	Name   string
	Kind   string
	Config json.RawMessage
}

func (s *Store) EnabledSources(ctx context.Context) ([]SourceConfig, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, name, kind, config FROM sources WHERE enabled = true ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SourceConfig
	for rows.Next() {
		var sc SourceConfig
		if err := rows.Scan(&sc.ID, &sc.Name, &sc.Kind, &sc.Config); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *Store) StartRun(ctx context.Context, sourceID int64) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO collector_runs (source_id, started_at, status)
		 VALUES ($1, now(), 'running') RETURNING id`, sourceID).Scan(&id)
	return id, err
}

func (s *Store) FinishRun(ctx context.Context, runID int64, status string, count int, errMsg string) error {
	var e *string
	if errMsg != "" {
		e = &errMsg
	}
	_, err := s.Pool.Exec(ctx,
		`UPDATE collector_runs SET finished_at = now(), status = $2, signal_count = $3, error = $4
		 WHERE id = $1`, runID, status, count, e)
	return err
}

// ---- Lookups loaded once at processor startup ----

// SourceIDs returns name -> id for all sources.
func (s *Store) SourceIDs(ctx context.Context) (map[string]int64, error) {
	return s.scanStringInt(ctx, `SELECT name, id FROM sources`)
}

// Aliases returns normalized alias -> entity_id (the resolution cascade's table).
func (s *Store) Aliases(ctx context.Context) (map[string]int64, error) {
	return s.scanStringInt(ctx, `SELECT alias, entity_id FROM entity_aliases`)
}

// Domains returns lowercased domain -> entity_id.
func (s *Store) Domains(ctx context.Context) (map[string]int64, error) {
	return s.scanStringInt(ctx, `SELECT lower(domain), id FROM entities WHERE domain IS NOT NULL AND domain <> ''`)
}

func (s *Store) scanStringInt(ctx context.Context, sql string) (map[string]int64, error) {
	rows, err := s.Pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// ---- Processor writes (transactional) ----

func (s *Store) SaveRawSignal(ctx context.Context, q DBTX, sourceID int64, sig signal.RawSignal) error {
	_, err := q.Exec(ctx,
		`INSERT INTO raw_signals (source_id, external_id, kind, event_time, observed_at, content_hash, payload)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sourceID, sig.ExternalID, string(sig.Kind), sig.EventTime, sig.ObservedAt, sig.ContentHash, sig.Payload)
	return err
}

// GetPosting returns the posting id and its latest content hash for a
// (source, external_id), or found=false if we have not seen it.
func (s *Store) GetPosting(ctx context.Context, q DBTX, sourceID int64, externalID string) (id int64, latestHash string, found bool, err error) {
	err = q.QueryRow(ctx,
		`SELECT p.id, COALESCE(v.content_hash, '')
		 FROM postings p
		 LEFT JOIN LATERAL (
		     SELECT content_hash FROM posting_versions
		     WHERE posting_id = p.id ORDER BY captured_at DESC LIMIT 1
		 ) v ON true
		 WHERE p.source_id = $1 AND p.external_id = $2`,
		sourceID, externalID).Scan(&id, &latestHash)
	if err == pgx.ErrNoRows {
		return 0, "", false, nil
	}
	return id, latestHash, err == nil, err
}

type NewPosting struct {
	EntityID   int64
	SourceID   int64
	ExternalID string
	Title      string
	URL        string
	Locations  []string
	Category   string
	IsSummer   bool
	FirstSeen  time.Time
	LastSeen   time.Time

	// Posted pay (nil / "" when the JD carries none). Extracted by the processor.
	PayMin      *float64
	PayMax      *float64
	PayPeriod   string
	PayCurrency string

	JDText string // plain-text JD (ATS sources only); "" -> NULL
}

func (s *Store) InsertPosting(ctx context.Context, q DBTX, p NewPosting) (int64, error) {
	var id int64
	err := q.QueryRow(ctx,
		`INSERT INTO postings (entity_id, source_id, external_id, title, url, locations, category, is_summer, first_seen, last_seen, status, pay_min, pay_max, pay_period, pay_currency, jd_text)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'open', $11, $12, $13, $14, $15) RETURNING id`,
		p.EntityID, p.SourceID, p.ExternalID, p.Title, p.URL, p.Locations, p.Category, p.IsSummer, p.FirstSeen, p.LastSeen,
		p.PayMin, p.PayMax, nullStr(p.PayPeriod), nullStr(p.PayCurrency), nullStr(p.JDText)).Scan(&id)
	return id, err
}

// nullStr maps "" to a nil *string so an empty value is stored as SQL NULL.
func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) UpdatePosting(ctx context.Context, q DBTX, id int64, title, url string, locations []string, lastSeen time.Time) error {
	_, err := q.Exec(ctx,
		`UPDATE postings SET title = $2, url = $3, locations = $4, last_seen = $5, status = 'open' WHERE id = $1`,
		id, title, url, locations, lastSeen)
	return err
}

func (s *Store) TouchPosting(ctx context.Context, q DBTX, id int64, lastSeen time.Time) error {
	_, err := q.Exec(ctx, `UPDATE postings SET last_seen = $2, status = 'open' WHERE id = $1`, id, lastSeen)
	return err
}

func (s *Store) InsertPostingVersion(ctx context.Context, q DBTX, postingID int64, hash, rawText string, parsed []byte, capturedAt time.Time) error {
	_, err := q.Exec(ctx,
		`INSERT INTO posting_versions (posting_id, content_hash, raw_text, parsed, captured_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		postingID, hash, rawText, parsed, capturedAt)
	return err
}

func (s *Store) InsertEvent(ctx context.Context, q DBTX, entityID int64, typ string, eventTime, ingestTime time.Time, postingID *int64, data []byte) (int64, error) {
	var id int64
	err := q.QueryRow(ctx,
		`INSERT INTO events (entity_id, type, event_time, ingest_time, posting_id, data)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		entityID, typ, eventTime, ingestTime, postingID, data).Scan(&id)
	return id, err
}

// RecordResolution appends to the resolution audit log. entityID nil = resolved
// to "not a watchlist entity".
func (s *Store) RecordResolution(ctx context.Context, q DBTX, rawText string, entityID *int64, method string, confidence float64, model string) error {
	var m *string
	if model != "" {
		m = &model
	}
	// resolution_decisions is a cache: one decision per raw string. ON CONFLICT
	// keeps the first decision and makes the write idempotent across restarts.
	_, err := q.Exec(ctx,
		`INSERT INTO resolution_decisions (raw_text, entity_id, method, confidence, model)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (raw_text) DO NOTHING`,
		rawText, entityID, method, confidence, m)
	return err
}
