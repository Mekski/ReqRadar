package store

import "context"

// This file holds the transactional-outbox queries (alert-loss-trio H1/H2) plus
// a transaction-aware variant of MarkFirehoseSeen. The pool-based MarkFirehoseSeen
// in api.go is retained for cmd/firehose-prime (which has no surrounding tx); the
// Tx variant here lets the processor commit "seen" and "staged for publish"
// atomically. (Worth unifying the two once concurrent edits to api.go settle.)

// OutboxRow is one staged-but-unpublished event awaiting delivery to NATS.
type OutboxRow struct {
	ID      int64
	Subject string
	Payload []byte
}

// InsertOutbox stages an event for publishing, in the same transaction as the
// DB writes it accompanies (the transactional-outbox pattern). Returns the row
// id so the caller can mark it published after a successful inline publish.
func (s *Store) InsertOutbox(ctx context.Context, q DBTX, subject string, payload []byte) (int64, error) {
	var id int64
	err := q.QueryRow(ctx,
		`INSERT INTO event_outbox (subject, payload) VALUES ($1, $2) RETURNING id`,
		subject, payload).Scan(&id)
	return id, err
}

// MarkOutboxPublished records that a staged event reached NATS, so the relay
// won't resend it.
func (s *Store) MarkOutboxPublished(ctx context.Context, q DBTX, id int64) error {
	_, err := q.Exec(ctx, `UPDATE event_outbox SET published_at = now() WHERE id = $1`, id)
	return err
}

// UnpublishedOutbox returns staged events not yet on NATS, oldest first — the
// relay's work list (events a failed inline publish left behind).
func (s *Store) UnpublishedOutbox(ctx context.Context, limit int) ([]OutboxRow, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, subject, payload FROM event_outbox
		 WHERE published_at IS NULL ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxRow
	for rows.Next() {
		var r OutboxRow
		if err := rows.Scan(&r.ID, &r.Subject, &r.Payload); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkFirehoseSeenTx is the transaction-aware variant of MarkFirehoseSeen: it
// records a firehose posting and returns true if it was new. Running it on the
// same tx as InsertOutbox makes "seen" and "will be published" commit together,
// closing the H1 drop-on-publish-failure gap.
func (s *Store) MarkFirehoseSeenTx(ctx context.Context, q DBTX, source, externalID, company, title, url, category string) (bool, error) {
	tag, err := q.Exec(ctx,
		`INSERT INTO firehose_seen (source, external_id, company, title, url, category)
		 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (source, external_id) DO NOTHING`,
		source, externalID, company, title, url, category)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
