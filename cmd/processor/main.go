// Command processor consumes signals.raw.*, normalizes and entity-resolves them,
// writes Postgres, and emits enriched events.<type>. See DESIGN.md §3.2.
package main

import (
	"strings"

	"github.com/Mekski/reqradar/internal/service"
)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("processor")
	defer stop()

	log.Info("starting", "nats", cfg.NATSURL, "postgres", redactDSN(cfg.PostgresDSN))

	// TODO(milestone-a): subscribe signals.raw.*, run normalize→resolve→diff→persist.
	<-ctx.Done()
	log.Info("shutting down")
}

// redactDSN hides credentials before logging a connection string.
func redactDSN(dsn string) string {
	if _, after, found := strings.Cut(dsn, "@"); found {
		return "postgres://***@" + after
	}
	return dsn
}
