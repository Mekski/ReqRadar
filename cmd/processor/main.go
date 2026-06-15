// Command processor consumes signals.raw.*, normalizes and entity-resolves them,
// writes Postgres, and emits enriched events.<type>. See DESIGN.md §3.2.
package main

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/processor"
	"github.com/Mekski/reqradar/internal/service"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("processor")
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

	proc, err := processor.New(ctx, st, b, log)
	if err != nil {
		log.Error("processor init", "err", err)
		return
	}

	// Durable push consumer over the SIGNALS stream. ManualAck so a failed
	// handle is redelivered (processing is idempotent).
	sub, err := b.SubscribeSignals(func(m *nats.Msg) {
		var raw signal.RawSignal
		if err := json.Unmarshal(m.Data, &raw); err != nil {
			log.Error("unmarshal raw signal", "err", err)
			_ = m.Term() // malformed: don't redeliver forever
			return
		}
		if err := proc.Handle(ctx, raw); err != nil {
			log.Error("handle signal", "source", raw.Source, "external_id", raw.ExternalID, "err", err)
			_ = m.Nak()
			return
		}
		_ = m.Ack()
	})
	if err != nil {
		log.Error("subscribe", "err", err)
		return
	}
	defer sub.Unsubscribe()

	// Periodically reload the resolver so dashboard-added companies (and seed
	// edits) take effect without a restart.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := proc.ReloadResolver(ctx); err != nil {
					log.Error("resolver reload", "err", err)
				}
			}
		}
	}()

	// Transactional-outbox relay: resend events a failed inline publish left
	// unpublished. Inline publish is the happy path (sub-second); this is the
	// backstop that guarantees a NATS hiccup never permanently drops an alert.
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := proc.RelayOutbox(ctx, 100); err != nil {
					log.Error("outbox relay", "err", err)
				}
			}
		}
	}()

	log.Info("processor consuming signals.raw.*")
	<-ctx.Done()
	log.Info("shutting down")
}
