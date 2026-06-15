// Package bus wraps NATS JetStream: connection, stream provisioning, and typed
// publish. Keeping it here means services never touch the nats client directly.
package bus

import (
	"errors"
	"time"

	"github.com/nats-io/nats.go"
)

// Subject helpers — the two subject hierarchies in the system.
const (
	SignalsPrefix = "signals.raw." // + <source>
	EventsPrefix  = "events."      // + <type>
)

type Bus struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

func Connect(url string) (*Bus, error) {
	nc, err := nats.Connect(url, nats.MaxReconnects(-1), nats.ReconnectWait(time.Second))
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, err
	}
	return &Bus{nc: nc, js: js}, nil
}

// EnsureStreams provisions the SIGNALS and EVENTS streams if absent. SIGNALS is
// the 30-day replay buffer (35d age cap = TTL + headroom); EVENTS is retained
// long (the processor is the thing that persists events forever in Postgres).
func (b *Bus) EnsureStreams() error {
	if err := b.ensureStream(&nats.StreamConfig{
		Name:      "SIGNALS",
		Subjects:  []string{SignalsPrefix + ">"},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    35 * 24 * time.Hour,
	}); err != nil {
		return err
	}
	return b.ensureStream(&nats.StreamConfig{
		Name:      "EVENTS",
		Subjects:  []string{EventsPrefix + ">"},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    90 * 24 * time.Hour,
	})
}

func (b *Bus) ensureStream(cfg *nats.StreamConfig) error {
	_, err := b.js.StreamInfo(cfg.Name)
	if errors.Is(err, nats.ErrStreamNotFound) {
		_, err = b.js.AddStream(cfg)
	}
	return err
}

// Publish sends data to a subject and waits for the JetStream ack.
func (b *Bus) Publish(subject string, data []byte) error {
	_, err := b.js.Publish(subject, data)
	return err
}

// SubscribeSignals registers a durable push consumer over signals.raw.*. Manual
// ack means a failed handle (Nak) is redelivered; processing is idempotent.
func (b *Bus) SubscribeSignals(handler nats.MsgHandler) (*nats.Subscription, error) {
	return b.js.Subscribe(SignalsPrefix+"*", handler,
		nats.Durable("processor"),
		nats.ManualAck(),
		nats.AckExplicit(),
	)
}

// SubscribeEvents registers a durable push consumer over events.*. DeliverNew
// means only events published after this consumer first starts are delivered —
// so historical/backfill events already in the stream never trigger alerts,
// while every genuinely new detection does. (A time-on-event_time filter would
// wrongly suppress newly-detected postings that carry an old date_posted.)
func (b *Bus) SubscribeEvents(handler nats.MsgHandler) (*nats.Subscription, error) {
	return b.js.Subscribe(EventsPrefix+"*", handler,
		nats.Durable("dispatcher"),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.DeliverNew(),
	)
}

func (b *Bus) Close() { _ = b.nc.Drain() }
