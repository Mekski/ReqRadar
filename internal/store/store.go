// Package store is the Postgres access layer shared by the services. It owns the
// connection pool and the queries; callers get typed methods, not raw SQL.
package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

// SourceConfig is a row from the sources table: a collector's enable flag and
// its JSON config (orgs, repo coordinates, etc.).
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

// StartRun records the beginning of a collector run and returns its id.
func (s *Store) StartRun(ctx context.Context, sourceID int64) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO collector_runs (source_id, started_at, status)
		 VALUES ($1, now(), 'running') RETURNING id`, sourceID).Scan(&id)
	return id, err
}

// FinishRun closes out a run. errMsg "" stores SQL NULL.
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
