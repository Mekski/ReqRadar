//go:build integration

// Integration test for the dedupe/diff state machine — the highest-risk logic in
// the processor (new → unchanged → changed), which is DB-coupled and so cannot be
// a pure unit test. It drives processor.Handle against a REAL Postgres and NATS
// and asserts the resulting rows.
//
// Safety: it is gated behind the `integration` build tag (plain `go test ./...`
// never compiles it) AND requires REQRADAR_TEST_DSN whose database name contains
// "test" — it refuses to run against the dev database with the 3-year backfill.
// Run with: go test -tags=integration ./internal/processor/...
package processor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
	"github.com/Mekski/reqradar/migrations"
)

// harness bundles the live dependencies plus the seeded watchlist entity.
type harness struct {
	st       *store.Store
	bus      *bus.Bus
	proc     *Processor
	entityID int64
}

// setup connects to the test Postgres + NATS, migrates, truncates, and seeds one
// watchlist entity ("Testco") with an alias and domain. Each test gets a clean
// slate. Skips (never fails the suite) when the test infra is absent or unsafe.
func setup(t *testing.T) *harness {
	t.Helper()

	dsn := os.Getenv("REQRADAR_TEST_DSN")
	if dsn == "" {
		t.Skip("REQRADAR_TEST_DSN not set — skipping integration test")
	}
	assertTestDB(t, dsn)

	natsURL := os.Getenv("REQRADAR_TEST_NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	ctx := context.Background()
	migrateUp(t, dsn)

	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(st.Close)

	truncate(t, st)
	entityID := seedEntity(t, st)

	b, err := bus.Connect(natsURL)
	if err != nil {
		t.Fatalf("bus.Connect(%s): %v", natsURL, err)
	}
	t.Cleanup(b.Close)
	if err := b.EnsureStreams(); err != nil {
		t.Fatalf("ensure streams: %v", err)
	}

	// proc.New loads the resolver tables + source ids, so seeding must precede it.
	proc, err := New(ctx, st, b, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("processor.New: %v", err)
	}
	return &harness{st: st, bus: b, proc: proc, entityID: entityID}
}

// assertTestDB refuses any DSN whose database name does not contain "test", so
// the suite can never truncate the dev database by accident.
func assertTestDB(t *testing.T, dsn string) {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse REQRADAR_TEST_DSN: %v", err)
	}
	db := strings.TrimPrefix(u.Path, "/")
	if !strings.Contains(strings.ToLower(db), "test") {
		t.Skipf("refusing to run: REQRADAR_TEST_DSN database %q does not contain 'test'", db)
	}
}

func migrateUp(t *testing.T, dsn string) {
	t.Helper()
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, strings.Replace(dsn, "postgres://", "pgx5://", 1))
	if err != nil {
		t.Fatalf("init migrate: %v", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
}

func truncate(t *testing.T, st *store.Store) {
	t.Helper()
	// CASCADE clears the watchlist/alias dependents; partitioned events/raw_signals
	// truncate via their parent.
	_, err := st.Pool.Exec(context.Background(), `
		TRUNCATE entities, entity_aliases, sources, postings, posting_versions,
		         events, raw_signals, resolution_decisions, firehose_seen, event_outbox
		RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func seedEntity(t *testing.T, st *store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	err := st.Pool.QueryRow(ctx,
		`INSERT INTO entities (kind, canonical_name, domain) VALUES ('company', 'Testco', 'testco.test') RETURNING id`,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed entity: %v", err)
	}
	if _, err := st.Pool.Exec(ctx,
		`INSERT INTO entity_aliases (entity_id, alias, source, confidence) VALUES ($1, 'testco', 'seed', 1.0)`, id,
	); err != nil {
		t.Fatalf("seed alias: %v", err)
	}
	// The source name must match a registered normalizer ("simplify-listings").
	if _, err := st.Pool.Exec(ctx,
		`INSERT INTO sources (name, kind, config, enabled) VALUES ('simplify-listings', 'aggregator', '{}', true)`,
	); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	return id
}

// listingPayload builds a simplify-listings wire payload (what normalizeSimplify
// parses). company drives entity resolution; category drives firehose routing.
func listingPayload(company, title, jdURL, category string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"company_name": company,
		"title":        title,
		"url":          jdURL,
		"locations":    []string{"San Francisco, CA"},
		"terms":        []string{"Summer 2027"},
		"category":     category,
	})
	return b
}

func sig(externalID, hash string, payload json.RawMessage, observedAt time.Time) signal.RawSignal {
	return signal.RawSignal{
		Source:      "simplify-listings",
		ExternalID:  externalID,
		Kind:        signal.KindPosting,
		EventTime:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		ObservedAt:  observedAt,
		Payload:     payload,
		ContentHash: hash,
	}
}

func countRows(t *testing.T, st *store.Store, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", sql, err)
	}
	return n
}

// TestProcessorStateMachine walks the dedupe/diff state machine end to end against
// real Postgres: a new posting opens, a re-delivery of the identical signal is a
// no-op touch (idempotency), and a changed content hash records a new version and
// emits jd_changed. (posting_closed is deferred per DESIGN §3.2, so not asserted.)
func TestProcessorStateMachine(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	const extID = "job-1"

	// --- 1. New posting -> posting_opened ---------------------------------
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	open := sig(extID, "hash-A", listingPayload("Testco", "SWE Intern", "https://testco.test/1", "Software Engineering"), t0)
	if err := h.proc.Handle(ctx, open); err != nil {
		t.Fatalf("handle new: %v", err)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM postings WHERE entity_id=$1`, h.entityID); got != 1 {
		t.Fatalf("postings after open = %d, want 1", got)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM posting_versions`); got != 1 {
		t.Fatalf("versions after open = %d, want 1", got)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM events WHERE type='posting_opened'`); got != 1 {
		t.Fatalf("posting_opened events = %d, want 1", got)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM raw_signals`); got != 1 {
		t.Fatalf("raw_signals after open = %d, want 1", got)
	}
	// The event was staged in the outbox and published inline (NATS is up), so
	// the row exists and is marked published (alert-loss-trio H2).
	if got := countRows(t, h.st, `SELECT count(*) FROM event_outbox`); got != 1 {
		t.Fatalf("event_outbox after open = %d, want 1", got)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM event_outbox WHERE published_at IS NOT NULL`); got != 1 {
		t.Errorf("outbox row should be published inline, got %d published", got)
	}

	// --- 2. Identical re-delivery -> idempotent touch, no new event -------
	t1 := t0.Add(5 * time.Minute)
	again := sig(extID, "hash-A", listingPayload("Testco", "SWE Intern", "https://testco.test/1", "Software Engineering"), t1)
	if err := h.proc.Handle(ctx, again); err != nil {
		t.Fatalf("handle redelivery: %v", err)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM posting_versions`); got != 1 {
		t.Errorf("versions after redelivery = %d, want 1 (no new version)", got)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM events`); got != 1 {
		t.Errorf("events after redelivery = %d, want 1 (no duplicate event)", got)
	}
	// last_seen must advance to the redelivery's observed time.
	var lastSeen time.Time
	if err := h.st.Pool.QueryRow(ctx, `SELECT last_seen FROM postings WHERE entity_id=$1`, h.entityID).Scan(&lastSeen); err != nil {
		t.Fatalf("read last_seen: %v", err)
	}
	if !lastSeen.UTC().Equal(t1) {
		t.Errorf("last_seen = %v, want touched to %v", lastSeen.UTC(), t1)
	}

	// --- 3. Changed content hash -> new version + jd_changed --------------
	t2 := t1.Add(5 * time.Minute)
	changed := sig(extID, "hash-B", listingPayload("Testco", "Senior SWE Intern", "https://testco.test/1", "Software Engineering"), t2)
	if err := h.proc.Handle(ctx, changed); err != nil {
		t.Fatalf("handle changed: %v", err)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM postings WHERE entity_id=$1`, h.entityID); got != 1 {
		t.Errorf("postings after change = %d, want still 1", got)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM posting_versions`); got != 2 {
		t.Errorf("versions after change = %d, want 2", got)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM events WHERE type='jd_changed'`); got != 1 {
		t.Errorf("jd_changed events = %d, want 1", got)
	}
	var title string
	if err := h.st.Pool.QueryRow(ctx, `SELECT title FROM postings WHERE entity_id=$1`, h.entityID).Scan(&title); err != nil {
		t.Fatalf("read title: %v", err)
	}
	if title != "Senior SWE Intern" {
		t.Errorf("title after change = %q, want updated", title)
	}
}

// TestProcessorFirehose verifies the non-watchlist path: a posting that does not
// resolve, in a firehose category, is recorded once (deduped) and not stored as a
// posting/event. A non-firehose category is ignored entirely.
func TestProcessorFirehose(t *testing.T) {
	h := setup(t)
	ctx := context.Background()

	// Unknown company + firehose category -> recorded once.
	fh := sig("fh-1", "hash-FH", listingPayload("Unknown Startup", "SWE Intern", "https://unknown.example/1", "Software Engineering"), time.Now())
	if err := h.proc.Handle(ctx, fh); err != nil {
		t.Fatalf("handle firehose: %v", err)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM firehose_seen`); got != 1 {
		t.Fatalf("firehose_seen after first = %d, want 1", got)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM postings`); got != 0 {
		t.Errorf("firehose must not create postings, got %d", got)
	}
	// Firehose event staged + published via the same outbox path (H1).
	if got := countRows(t, h.st, `SELECT count(*) FROM event_outbox WHERE subject='events.firehose' AND published_at IS NOT NULL`); got != 1 {
		t.Errorf("firehose outbox row should be published, got %d", got)
	}

	// Re-delivery -> deduped, still one row.
	if err := h.proc.Handle(ctx, fh); err != nil {
		t.Fatalf("handle firehose redelivery: %v", err)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM firehose_seen`); got != 1 {
		t.Errorf("firehose_seen after redelivery = %d, want 1 (deduped)", got)
	}

	// Non-firehose category -> ignored.
	other := sig("fh-2", "hash-X", listingPayload("Another Co", "Marketing Intern", "https://another.example/1", "Marketing"), time.Now())
	if err := h.proc.Handle(ctx, other); err != nil {
		t.Fatalf("handle non-firehose: %v", err)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM firehose_seen`); got != 1 {
		t.Errorf("non-firehose category should be ignored, firehose_seen = %d, want 1", got)
	}
}

// TestProcessorOutboxRelay verifies the transactional-outbox backstop (H1/H2):
// an event a failed inline publish left unpublished is resent by RelayOutbox,
// exactly once (a second sweep is a no-op — no duplicate).
func TestProcessorOutboxRelay(t *testing.T) {
	h := setup(t)
	ctx := context.Background()

	// Create a posting; its event is staged and published inline.
	open := sig("relay-1", "hash-R", listingPayload("Testco", "SWE Intern", "https://testco.test/r", "Software Engineering"), time.Now())
	if err := h.proc.Handle(ctx, open); err != nil {
		t.Fatalf("handle: %v", err)
	}

	// Simulate a failed inline publish: reset the row to unpublished (a straggler).
	if _, err := h.st.Pool.Exec(ctx, `UPDATE event_outbox SET published_at = NULL`); err != nil {
		t.Fatalf("reset outbox: %v", err)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM event_outbox WHERE published_at IS NULL`); got != 1 {
		t.Fatalf("setup: want 1 unpublished row, got %d", got)
	}

	// First sweep republishes it and marks it published.
	n, err := h.proc.RelayOutbox(ctx, 100)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	if n != 1 {
		t.Errorf("first relay sweep republished %d, want 1", n)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM event_outbox WHERE published_at IS NULL`); got != 0 {
		t.Errorf("after relay, unpublished rows = %d, want 0", got)
	}

	// Second sweep is a no-op — the row is published, so it is not resent (no dup).
	n, err = h.proc.RelayOutbox(ctx, 100)
	if err != nil {
		t.Fatalf("relay 2: %v", err)
	}
	if n != 0 {
		t.Errorf("second relay sweep republished %d, want 0 (no duplicate)", n)
	}
}

// TestRecordResolutionDedup verifies resolution_decisions is a cache: recording
// the same raw string twice (as a processor restart would) keeps exactly one row,
// thanks to the unique index + ON CONFLICT DO NOTHING. Without it, every restart
// re-appended rows for the whole feed into a kept-forever table.
func TestRecordResolutionDedup(t *testing.T) {
	h := setup(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := h.st.RecordResolution(ctx, h.st.Pool, "Some Startup", nil, "none", 0.0, ""); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM resolution_decisions WHERE raw_text = 'Some Startup'`); got != 1 {
		t.Errorf("resolution_decisions rows for repeated string = %d, want 1 (cache)", got)
	}

	// A different string still gets its own row.
	eid := h.entityID
	if err := h.st.RecordResolution(ctx, h.st.Pool, "Testco", &eid, "alias", 1.0, ""); err != nil {
		t.Fatalf("record distinct: %v", err)
	}
	if got := countRows(t, h.st, `SELECT count(*) FROM resolution_decisions`); got != 2 {
		t.Errorf("total resolution_decisions = %d, want 2 (one per distinct string)", got)
	}
}
