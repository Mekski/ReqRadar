package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
)

// Factory builds a collector from its source-row config. Registering factories
// (not instances) lets the DB sources table drive which collectors exist and how
// they're configured. The store is passed so a collector can read live operational
// config from the DB (e.g. the ATS collectors read which board slugs to poll from
// the watchlist each cycle); aggregators that don't need it just ignore it.
// See DESIGN.md §3.1.
type Factory func(cfg json.RawMessage, st *store.Store, log *slog.Logger) (Collector, error)

// Runner schedules registered collectors against the enabled sources in the DB.
type Runner struct {
	bus       *bus.Bus
	store     *store.Store
	log       *slog.Logger
	factories map[string]Factory
}

func NewRunner(b *bus.Bus, st *store.Store, log *slog.Logger) *Runner {
	return &Runner{bus: b, store: st, log: log, factories: map[string]Factory{}}
}

func (r *Runner) Register(name string, f Factory) { r.factories[name] = f }

// Run starts one goroutine per enabled source that has a registered factory and
// blocks until ctx is cancelled. Enabled sources with no factory are logged and
// skipped (expected while collectors are still being built out).
func (r *Runner) Run(ctx context.Context) error {
	sources, err := r.store.EnabledSources(ctx)
	if err != nil {
		return fmt.Errorf("load sources: %w", err)
	}
	started := 0
	for _, s := range sources {
		f, ok := r.factories[s.Name]
		if !ok {
			r.log.Warn("enabled source has no collector registered yet", "source", s.Name)
			continue
		}
		c, err := f(s.Config, r.store, r.log.With("collector", s.Name))
		if err != nil {
			r.log.Error("collector init failed", "source", s.Name, "err", err)
			continue
		}
		go r.loop(ctx, c, s.ID)
		started++
	}
	r.log.Info("collector runner started", "running", started, "enabled_sources", len(sources))
	<-ctx.Done()
	return nil
}

func (r *Runner) loop(ctx context.Context, c Collector, sourceID int64) {
	ticker := time.NewTicker(c.Schedule())
	defer ticker.Stop()

	var since time.Time // zero on first run; collectors that don't need it ignore it
	run := func() {
		if err := r.runOnce(ctx, c, sourceID, since); err != nil {
			r.log.Error("collector run failed", "collector", c.Name(), "err", err)
		}
		since = time.Now()
	}

	run() // run immediately on startup, then on schedule
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// runOnce executes a single collection, records it in collector_runs, and
// publishes the resulting signals. A panic in a collector is recovered here so a
// single bad source can never take down the others or the service.
func (r *Runner) runOnce(ctx context.Context, c Collector, sourceID int64, since time.Time) (err error) {
	runID, err := r.store.StartRun(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("start run: %w", err)
	}

	var signals []signal.RawSignal
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
		status, msg, count := "ok", "", len(signals)
		if err != nil {
			status, msg, count = "error", err.Error(), 0
		}
		if ferr := r.store.FinishRun(ctx, runID, status, count, msg); ferr != nil {
			r.log.Error("finish run failed", "collector", c.Name(), "err", ferr)
		}
	}()

	signals, err = c.Collect(ctx, since)
	if err != nil {
		return err
	}

	subject := bus.SignalsPrefix + c.Name()
	published := 0
	for i := range signals {
		data, merr := json.Marshal(signals[i])
		if merr != nil {
			r.log.Error("marshal signal", "collector", c.Name(), "err", merr)
			continue
		}
		if perr := r.bus.Publish(subject, data); perr != nil {
			return fmt.Errorf("publish: %w", perr)
		}
		published++
	}
	r.log.Info("collected", "collector", c.Name(), "published", published)
	return nil
}
