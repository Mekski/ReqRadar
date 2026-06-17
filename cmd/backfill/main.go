// Command backfill reconstructs historical posting-timing data by replaying
// git-history snapshots of the SimplifyJobs listings through the normal pipeline.
// It shares its orchestration with the dashboard's "rebuild history" button via
// internal/backfill. Run the processor alongside this. See DESIGN.md §5 (backfill).
package main

import (
	"github.com/Mekski/reqradar/internal/backfill"
	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/service"
	"github.com/Mekski/reqradar/internal/store"
)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("backfill")
	defer stop()

	st, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Error("store open", "err", err)
		return
	}
	defer st.Close()

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

	if err := backfill.NewRunner(st, b, log).Run(ctx); err != nil {
		log.Error("backfill", "err", err)
	}
}
