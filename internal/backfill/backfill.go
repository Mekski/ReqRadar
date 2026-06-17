// Package backfill replays the SimplifyJobs git-history snapshots through the
// normal pipeline (emit historical RawSignals to signals.raw.simplify-listings;
// the running processor resolves/dedupes/persists them with their original
// event_time). It is the one orchestration shared by the `backfill` CLI and the
// dashboard's "rebuild history" button, so there's a single definition.
//
// It is idempotent (the processor dedupes by external_id+content_hash) and never
// floods alerts (backfill signals skip the firehose, and DeliverNew + the 48h
// freshness gate suppress watchlist alerts). A run takes ~30s.
package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/collector"
	"github.com/Mekski/reqradar/internal/collector/simplify"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
)

// Start is the git origin of listings.json (verified 2023-08-02).
var Start = time.Date(2023, 8, 1, 0, 0, 0, 0, time.UTC)

// ErrAlreadyRunning is returned when a backfill is requested while one is in flight.
var ErrAlreadyRunning = errors.New("a backfill is already running")

// Status is the observable state of the runner (exposed to the dashboard).
type Status struct {
	Running   bool       `json:"running"`
	LastRunAt *time.Time `json:"last_run_at"`
	LastError string     `json:"last_error,omitempty"`
	Emitted   int        `json:"emitted"` // postings replayed on the last completed run
}

type Runner struct {
	store *store.Store
	bus   *bus.Bus
	log   *slog.Logger

	mu     sync.Mutex
	status Status
}

func NewRunner(st *store.Store, b *bus.Bus, log *slog.Logger) *Runner {
	return &Runner{store: st, bus: b, log: log}
}

// Status returns a snapshot of the runner state.
func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// Run replays the full history once. It blocks until done (~30s); callers that
// don't want to wait should invoke it in a goroutine. Concurrent runs are rejected
// with ErrAlreadyRunning so a double-click can't launch two.
func (r *Runner) Run(ctx context.Context) error {
	r.mu.Lock()
	if r.status.Running {
		r.mu.Unlock()
		return ErrAlreadyRunning
	}
	r.status.Running = true
	r.status.LastError = ""
	r.mu.Unlock()

	emitted, err := r.replay(ctx)

	r.mu.Lock()
	now := time.Now()
	r.status.Running = false
	r.status.LastRunAt = &now
	r.status.Emitted = emitted
	if err != nil {
		r.status.LastError = err.Error()
	}
	r.mu.Unlock()

	if err != nil {
		r.log.Error("backfill failed", "err", err)
		return err
	}
	r.log.Info("backfill done — processor will drain the signals", "emitted", emitted)
	return nil
}

// replay builds the SimplifyJobs Backfiller from its source config and runs it,
// publishing each historical signal to NATS. Returns the count emitted.
func (r *Runner) replay(ctx context.Context) (int, error) {
	sources, err := r.store.EnabledSources(ctx)
	if err != nil {
		return 0, fmt.Errorf("load sources: %w", err)
	}
	var cfg json.RawMessage
	for _, s := range sources {
		if s.Name == "simplify-listings" {
			cfg = s.Config
		}
	}
	if cfg == nil {
		return 0, errors.New("simplify-listings source not found/enabled")
	}

	c, err := simplify.New(cfg, r.store, r.log)
	if err != nil {
		return 0, fmt.Errorf("simplify init: %w", err)
	}
	bf, ok := c.(collector.Backfiller)
	if !ok {
		return 0, errors.New("simplify collector does not support backfill")
	}

	subject := bus.SignalsPrefix + c.Name()
	emitted := 0
	emit := func(sig signal.RawSignal) error {
		data, err := json.Marshal(sig)
		if err != nil {
			return err
		}
		if err := r.bus.Publish(subject, data); err != nil {
			return err
		}
		emitted++
		return nil
	}

	r.log.Info("backfill starting", "from", Start.Format("2006-01-02"))
	if err := bf.Backfill(ctx, Start, time.Now(), emit); err != nil {
		return emitted, fmt.Errorf("backfill: %w", err)
	}
	return emitted, nil
}
