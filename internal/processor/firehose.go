package processor

import (
	"context"
	"encoding/json"

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
func (p *Processor) maybeFirehose(ctx context.Context, raw signal.RawSignal, post Posting) error {
	if !firehoseCategories[post.Category] {
		return nil
	}
	isNew, err := p.store.MarkFirehoseSeen(ctx, raw.Source, raw.ExternalID, post.Company, post.Title, post.URL, post.Category)
	if err != nil {
		return err
	}
	if !isNew {
		return nil
	}
	data, _ := json.Marshal(map[string]any{
		"company": post.Company, "title": post.Title, "url": post.URL, "category": post.Category,
	})
	p.publish(signal.Event{Type: "firehose", ObservedAt: raw.ObservedAt, Data: data})
	return nil
}
