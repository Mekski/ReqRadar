// Command collector runs all collector plugins on their schedules and publishes
// RawSignals to NATS subject signals.raw.<source>. See DESIGN.md §3.1.
package main

import (
	"github.com/Mekski/reqradar/internal/service"
)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("collector")
	defer stop()

	log.Info("starting", "nats", cfg.NATSURL)

	// TODO(milestone-a): connect NATS, register collector plugins, run scheduler.
	<-ctx.Done()
	log.Info("shutting down")
}
