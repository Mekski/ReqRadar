package processor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/signal"
)

// firehoseCategories is the SWE + AI/ML scope chosen for broad (non-watchlist)
// alerts. The aggregator's category taxonomy is messy — several labels mean the
// same thing — hence the multiple keys. Watchlist companies are NOT category-
// filtered (you want every role from your top 15); the firehose is, because its
// volume is ~1000s.
var firehoseCategories = map[string]bool{
	"Software":                            true,
	"Software Engineering":                true,
	"AI/ML/Data":                          true,
	"Data Science, AI & Machine Learning": true,
}

// maybeFirehose handles a posting that did NOT resolve to a watchlist entity: if
// it's in the firehose categories and we haven't seen it, emit an events.firehose
// for the dispatcher. No entity/posting/event rows — firehose is alert-only.
//
// The "seen" mark and the outbox insert run in one transaction (alert-loss-trio
// H1): otherwise a publish failure after marking-seen would drop the alert and
// the dedup would suppress it forever. Inline publish after commit keeps latency
// sub-second; the relay backstops a failed publish.
func (p *Processor) maybeFirehose(ctx context.Context, raw signal.RawSignal, post Posting) error {
	if !firehoseCategories[post.Category] {
		return nil
	}

	tx, err := p.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	isNew, err := p.store.MarkFirehoseSeenTx(ctx, tx, raw.Source, raw.ExternalID, post.Company, post.Title, post.URL, post.Category, raw.EventTime)
	if err != nil {
		return err
	}
	if !isNew {
		return nil // already seen — rolled back by the deferred Rollback
	}

	data, _ := json.Marshal(map[string]any{
		"company": post.Company, "title": post.Title, "url": post.URL, "category": post.Category,
	})
	payload, _ := json.Marshal(signal.Event{Type: "firehose", EventTime: raw.EventTime, ObservedAt: raw.ObservedAt, Data: data})
	subject := bus.EventsPrefix + "firehose"
	outboxID, err := p.store.InsertOutbox(ctx, tx, subject, payload)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit firehose: %w", err)
	}

	p.publishStaged(ctx, outboxID, subject, payload)
	return nil
}
