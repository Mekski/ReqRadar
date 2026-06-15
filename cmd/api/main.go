// Command api serves the dashboard REST API and runs the alert dispatcher: an
// events.* consumer that filters against the watchlist and sends Telegram
// alerts, recording detect_to_alert_ms per alert. See DESIGN.md §3.3.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/Mekski/reqradar/internal/api"
	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/service"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
	"github.com/Mekski/reqradar/internal/telegram"
)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("api")
	defer stop()

	st, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Error("store open", "err", err)
		return
	}
	defer st.Close()

	userID, err := st.FirstUserID(ctx)
	if err != nil {
		log.Error("no user found — run the seed first", "err", err)
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

	// Alert dispatcher: consume events.* (DeliverNew) and fire Telegram alerts.
	if cfg.TelegramToken == "" {
		log.Warn("TELEGRAM_BOT_TOKEN not set — alerts will fail to send")
	}
	disp := api.NewDispatcher(st, telegram.New(cfg.TelegramToken), log)
	sub, err := b.SubscribeEvents(func(m *nats.Msg) {
		var e signal.Event
		if err := json.Unmarshal(m.Data, &e); err != nil {
			log.Error("unmarshal event", "err", err)
			_ = m.Term()
			return
		}
		if err := disp.Handle(ctx, e); err != nil {
			log.Error("dispatch", "event_id", e.EventID, "err", err)
			_ = m.Nak()
			return
		}
		_ = m.Ack()
	})
	if err != nil {
		log.Error("subscribe events", "err", err)
		return
	}
	defer sub.Unsubscribe()

	// REST API for the dashboard.
	srv := &http.Server{Addr: cfg.APIAddr, Handler: api.NewServer(st, log, userID)}
	go func() {
		log.Info("api listening", "addr", cfg.APIAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}
