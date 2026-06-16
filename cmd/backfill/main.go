// Command backfill reconstructs historical posting-timing data by replaying
// git-history snapshots of the SimplifyJobs listings through the normal pipeline:
// it emits historical RawSignals to signals.raw.simplify-listings, and the
// running processor resolves/dedupes/persists them with their original
// event_time. Run the processor alongside this. See DESIGN.md §5 (backfill).
package main

import (
	"encoding/json"
	"time"

	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/collector"
	"github.com/Mekski/reqradar/internal/collector/simplify"
	"github.com/Mekski/reqradar/internal/service"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
)

// backfillStart is the git origin of listings.json (verified 2023-08-02).
var backfillStart = time.Date(2023, 8, 1, 0, 0, 0, 0, time.UTC)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("backfill")
	defer stop()

	st, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Error("store open", "err", err)
		return
	}
	defer st.Close()

	sources, err := st.EnabledSources(ctx)
	if err != nil {
		log.Error("load sources", "err", err)
		return
	}
	var simplifyCfg json.RawMessage
	for _, s := range sources {
		if s.Name == "simplify-listings" {
			simplifyCfg = s.Config
		}
	}
	if simplifyCfg == nil {
		log.Error("simplify-listings source not found/enabled")
		return
	}

	b, err := bus.Connect(cfg.NATSURL)
	if err != nil {
		log.Error("nats connect", "err", err)
		return
	}
	defer b.Close()
	if err := b.EnsureStreams(); err != nil {
		log.Error("ensure streams", "err", err)
		return
	}

	c, err := simplify.New(simplifyCfg, st, log)
	if err != nil {
		log.Error("simplify init", "err", err)
		return
	}
	bf, ok := c.(collector.Backfiller)
	if !ok {
		log.Error("simplify collector does not support backfill")
		return
	}

	subject := bus.SignalsPrefix + c.Name()
	emit := func(sig signal.RawSignal) error {
		data, err := json.Marshal(sig)
		if err != nil {
			return err
		}
		return b.Publish(subject, data)
	}

	log.Info("backfill starting", "from", backfillStart.Format("2006-01-02"))
	if err := bf.Backfill(ctx, backfillStart, time.Now(), emit); err != nil {
		log.Error("backfill failed", "err", err)
		return
	}
	log.Info("backfill done — processor will drain the signals")
}
