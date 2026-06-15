// Command collector runs all registered collector plugins on their schedules and
// publishes RawSignals to NATS subject signals.raw.<source>. See DESIGN.md §3.1.
package main

import (
	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/collector"
	"github.com/Mekski/reqradar/internal/collector/greenhouse"
	"github.com/Mekski/reqradar/internal/collector/simplify"
	"github.com/Mekski/reqradar/internal/service"
	"github.com/Mekski/reqradar/internal/store"
)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("collector")
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

	r := collector.NewRunner(b, st, log)
	// Register collector factories. Adding a source = one line here + a sources
	// row; the DB's enabled flag decides whether it actually runs.
	r.Register("simplify-listings", simplify.New)
	r.Register("greenhouse", greenhouse.New)

	if err := r.Run(ctx); err != nil {
		log.Error("runner stopped", "err", err)
	}
	log.Info("shutting down")
}
