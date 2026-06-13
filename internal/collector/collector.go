// Package collector defines the plugin contract for signal sources. Adding a
// source is implementing Collector in one file plus a sources config row — that
// cheapness is the point of the framework. See DESIGN.md §3.1.
package collector

import (
	"context"
	"time"

	"github.com/Mekski/reqradar/internal/signal"
)

// Collector is the plugin contract every source implements.
type Collector interface {
	// Name is the stable source identifier, used as the NATS subject suffix
	// (signals.raw.<name>) and the sources config key.
	Name() string

	// Schedule is how often Collect should run.
	Schedule() time.Duration

	// Collect fetches signals observed since the given time. Implementations
	// fetch, stamp, hash, and return; they must not parse semantics.
	Collect(ctx context.Context, since time.Time) ([]signal.RawSignal, error)
}

// Backfiller is an optional capability for collectors that can reconstruct
// history, e.g. the SimplifyJobs collector walking listings.json git history.
// EventTime on emitted signals is the historical timestamp; ObservedAt is now.
type Backfiller interface {
	Backfill(ctx context.Context, from, to time.Time, emit func(signal.RawSignal) error) error
}
