// Command api serves the dashboard REST API and runs the alert dispatcher: an
// events.* consumer that filters against the watchlist and sends Telegram
// alerts, recording detect_to_alert_ms per alert. See DESIGN.md §3.3.
package main

import (
	"github.com/Mekski/reqradar/internal/service"
)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("api")
	defer stop()

	log.Info("starting", "nats", cfg.NATSURL)

	// TODO(milestone-b): start HTTP server + events.* alert consumer.
	<-ctx.Done()
	log.Info("shutting down")
}
