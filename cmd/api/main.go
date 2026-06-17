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
	"github.com/Mekski/reqradar/internal/ats"
	"github.com/Mekski/reqradar/internal/backfill"
	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/expected"
	"github.com/Mekski/reqradar/internal/fit"
	"github.com/Mekski/reqradar/internal/llm"
	"github.com/Mekski/reqradar/internal/sentiment"
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

	// Fit score (LLM): free-tier Gemini, reached only on-demand from the API.
	if cfg.GeminiKey == "" {
		log.Warn("GEMINI_API_KEY not set — fit scoring will return 'not configured' until it is")
	}
	gemini := llm.NewGemini(cfg.GeminiKey, cfg.GeminiModel)
	fitSvc := fit.New(gemini, st)
	sentSvc := sentiment.New(gemini, st)
	expSvc := expected.New(gemini, st)
	atsSvc := ats.New(gemini, st)
	bfRunner := backfill.NewRunner(st, b, log)

	// REST API for the dashboard. Timeouts guard against slow/abandoned clients
	// once this is internet-reachable; WriteTimeout sits above the LLM call budget
	// so on-demand fit/sentiment responses aren't cut off mid-flight.
	srv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler: api.NewServer(st, log, userID, fitSvc, sentSvc, expSvc, atsSvc, bfRunner, api.ServerConfig{
			APIToken: cfg.APIToken, CORSOrigin: cfg.CORSOrigin,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
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
