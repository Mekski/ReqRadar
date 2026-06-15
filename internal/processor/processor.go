// Package processor consumes raw signals, normalizes and entity-resolves them,
// dedupes against stored state, persists postings/versions/events, and emits
// events.<type> for the alert dispatcher. See DESIGN.md §3.2.
package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Mekski/reqradar/internal/bus"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
)

type Processor struct {
	store       *store.Store
	bus         *bus.Bus
	log         *slog.Logger
	resolver    *Resolver
	normalizers map[string]Normalizer
	sourceIDs   map[string]int64

	mu       sync.Mutex
	recorded map[string]bool // raw company text -> resolution decision already logged
}

// New loads the resolver tables and source ids from the DB and wires the
// per-source normalizers.
func New(ctx context.Context, st *store.Store, b *bus.Bus, log *slog.Logger) (*Processor, error) {
	aliases, err := st.Aliases(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aliases: %w", err)
	}
	domains, err := st.Domains(ctx)
	if err != nil {
		return nil, fmt.Errorf("load domains: %w", err)
	}
	sourceIDs, err := st.SourceIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("load source ids: %w", err)
	}
	return &Processor{
		store:       st,
		bus:         b,
		log:         log,
		resolver:    NewResolver(aliases, domains),
		normalizers: map[string]Normalizer{"simplify-listings": normalizeSimplify},
		sourceIDs:   sourceIDs,
		recorded:    map[string]bool{},
	}, nil
}

// Handle processes one raw signal. It is idempotent: re-delivery of an already
// stored signal results in a last_seen touch, not a duplicate event, because
// change detection reads committed posting state.
func (p *Processor) Handle(ctx context.Context, raw signal.RawSignal) error {
	sourceID, ok := p.sourceIDs[raw.Source]
	if !ok {
		return fmt.Errorf("unknown source %q", raw.Source)
	}
	normalize, ok := p.normalizers[raw.Source]
	if !ok {
		return fmt.Errorf("no normalizer for source %q", raw.Source)
	}

	post, err := normalize(raw.Payload)
	if err != nil {
		return fmt.Errorf("normalize: %w", err)
	}

	entityID, method, resolved := p.resolver.Resolve(post.Company, post.URL)
	p.recordDecision(ctx, post.Company, entityID, method, resolved)
	if !resolved {
		// Not a watchlist company — route to the firehose (broad alerts).
		return p.maybeFirehose(ctx, raw, post)
	}

	return p.persist(ctx, raw, sourceID, entityID, post)
}

// persist runs the dedupe + write + emit for a watchlist-resolved signal in a
// single transaction, then publishes the event after commit.
func (p *Processor) persist(ctx context.Context, raw signal.RawSignal, sourceID, entityID int64, post Posting) error {
	tx, err := p.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	id, latestHash, found, err := p.store.GetPosting(ctx, tx, sourceID, raw.ExternalID)
	if err != nil {
		return fmt.Errorf("get posting: %w", err)
	}

	parsed, _ := json.Marshal(post)
	var emit *signal.Event

	switch {
	case !found:
		if err := p.store.SaveRawSignal(ctx, tx, sourceID, raw); err != nil {
			return err
		}
		pid, err := p.store.InsertPosting(ctx, tx, store.NewPosting{
			EntityID: entityID, SourceID: sourceID, ExternalID: raw.ExternalID,
			Title: post.Title, URL: post.URL, Locations: post.Locations,
			FirstSeen: raw.ObservedAt, LastSeen: raw.ObservedAt,
		})
		if err != nil {
			return fmt.Errorf("insert posting: %w", err)
		}
		if err := p.store.InsertPostingVersion(ctx, tx, pid, raw.ContentHash, string(raw.Payload), parsed, raw.ObservedAt); err != nil {
			return err
		}
		emit, err = p.makeEvent(ctx, tx, entityID, "posting_opened", raw, &pid, post)
		if err != nil {
			return err
		}

	case latestHash != raw.ContentHash:
		if err := p.store.SaveRawSignal(ctx, tx, sourceID, raw); err != nil {
			return err
		}
		if err := p.store.InsertPostingVersion(ctx, tx, id, raw.ContentHash, string(raw.Payload), parsed, raw.ObservedAt); err != nil {
			return err
		}
		if err := p.store.UpdatePosting(ctx, tx, id, post.Title, post.URL, post.Locations, raw.ObservedAt); err != nil {
			return err
		}
		emit, err = p.makeEvent(ctx, tx, entityID, "jd_changed", raw, &id, post)
		if err != nil {
			return err
		}

	default: // unchanged — just record that it is still open
		if err := p.store.TouchPosting(ctx, tx, id, raw.ObservedAt); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if emit != nil {
		p.publish(*emit)
	}
	return nil
}

func (p *Processor) makeEvent(ctx context.Context, tx store.DBTX, entityID int64, typ string, raw signal.RawSignal, postingID *int64, post Posting) (*signal.Event, error) {
	data, _ := json.Marshal(map[string]any{
		"title": post.Title, "url": post.URL, "company": post.Company, "locations": post.Locations,
	})
	eventID, err := p.store.InsertEvent(ctx, tx, entityID, typ, raw.EventTime, time.Now(), postingID, data)
	if err != nil {
		return nil, fmt.Errorf("insert event: %w", err)
	}
	return &signal.Event{
		EventID: eventID, EntityID: entityID, PostingID: postingID, Type: typ,
		EventTime: raw.EventTime, ObservedAt: raw.ObservedAt, Data: data,
	}, nil
}

func (p *Processor) publish(e signal.Event) {
	data, err := json.Marshal(e)
	if err != nil {
		p.log.Error("marshal event", "err", err)
		return
	}
	if err := p.bus.Publish(bus.EventsPrefix+e.Type, data); err != nil {
		// The event is committed in Postgres (the source of truth); a failed
		// publish only delays the alert. Logged, not fatal.
		p.log.Error("publish event", "type", e.Type, "event_id", e.EventID, "err", err)
	}
}

// recordDecision logs each unique raw company string to the resolution audit
// once per process run, keeping the table at ~unique-company size, not per-signal.
func (p *Processor) recordDecision(ctx context.Context, company string, entityID int64, method string, resolved bool) {
	p.mu.Lock()
	if p.recorded[company] {
		p.mu.Unlock()
		return
	}
	p.recorded[company] = true
	p.mu.Unlock()

	var eid *int64
	conf := 0.0
	if resolved {
		eid = &entityID
		conf = 1.0
		if method == "domain" {
			conf = 0.8
		}
	}
	if err := p.store.RecordResolution(ctx, p.store.Pool, company, eid, method, conf, ""); err != nil {
		p.log.Error("record resolution", "company", company, "err", err)
	}
}
