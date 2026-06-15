//go:build integration

// Integration test for the consumer redelivery policy (alert-loss-trio H3).
// It verifies that SubscribeSignals/SubscribeEvents create their durable
// consumers with the MaxDeliver / AckWait / BackOff config — the cap that stops
// a poison message from being redelivered forever.
//
// Requires a throwaway NATS (REQRADAR_TEST_NATS_URL, default localhost:4222).
// It deletes and recreates the "processor"/"dispatcher" durables, so it must NOT
// be pointed at a NATS with a live processor/api attached. Run with:
//
//	go test -tags=integration ./internal/bus/...
package bus

import (
	"os"
	"reflect"
	"testing"

	"github.com/nats-io/nats.go"
)

func TestConsumerRedeliveryConfig(t *testing.T) {
	url := os.Getenv("REQRADAR_TEST_NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}

	b, err := Connect(url)
	if err != nil {
		t.Skipf("no NATS at %s — skipping (%v)", url, err)
	}
	defer b.Close()
	if err := b.EnsureStreams(); err != nil {
		t.Fatalf("ensure streams: %v", err)
	}

	// Admin connection to clean slate + read back consumer config. js.Subscribe
	// binds to (does not update) an existing durable, so we delete first to force
	// a fresh create with the current options.
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	cases := []struct {
		name      string
		stream    string
		durable   string
		subscribe func(nats.MsgHandler) (*nats.Subscription, error)
	}{
		{"signals", "SIGNALS", "processor", b.SubscribeSignals},
		{"events", "EVENTS", "dispatcher", b.SubscribeEvents},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_ = js.DeleteConsumer(c.stream, c.durable) // ignore "not found"
			t.Cleanup(func() { _ = js.DeleteConsumer(c.stream, c.durable) })

			sub, err := c.subscribe(func(*nats.Msg) {})
			if err != nil {
				t.Fatalf("subscribe: %v", err)
			}
			defer sub.Unsubscribe()

			info, err := js.ConsumerInfo(c.stream, c.durable)
			if err != nil {
				t.Fatalf("consumer info: %v", err)
			}
			if info.Config.MaxDeliver != maxDeliver {
				t.Errorf("MaxDeliver = %d, want %d", info.Config.MaxDeliver, maxDeliver)
			}
			if info.Config.AckWait != ackWait {
				t.Errorf("AckWait = %v, want %v", info.Config.AckWait, ackWait)
			}
			if !reflect.DeepEqual(info.Config.BackOff, redeliveryBackoff) {
				t.Errorf("BackOff = %v, want %v", info.Config.BackOff, redeliveryBackoff)
			}
		})
	}
}
