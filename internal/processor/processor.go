// Package processor consumes raw signals, normalizes and entity-resolves them,
// dedupes against stored state, persists postings/versions/events, and emits
// events.<type> for the alert dispatcher. See DESIGN.md §3.2.
package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
		normalizers: map[string]Normalizer{"simplify-listings": normalizeSimplify, "greenhouse": normalizeGreenhouse, "ashby": normalizeAshby},
		sourceIDs:   sourceIDs,
		recorded:    map[string]bool{},
	}, nil
}

// ReloadResolver refreshes the resolver's alias/domain tables from the DB, so
// companies added via the dashboard (or edits to the seed) take effect without a
// restart.
func (p *Processor) ReloadResolver(ctx context.Context) error {
	aliases, err := p.store.Aliases(ctx)
	if err != nil {
		return err
	}
	domains, err := p.store.Domains(ctx)
	if err != nil {
		return err
	}
	p.resolver.Reload(aliases, domains)
	return nil
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
	isSummer := false
	for _, t := range post.Terms {
		if strings.HasPrefix(t, "Summer") {
			isSummer = true
			break
		}
	}
	var payMin, payMax *float64
	if post.PayPeriod != "" {
		payMin, payMax = &post.PayMin, &post.PayMax
	}
	var emit *signal.Event

	switch {
	case !found:
		if err := p.store.SaveRawSignal(ctx, tx, sourceID, raw); err != nil {
			return err
		}
		pid, err := p.store.InsertPosting(ctx, tx, store.NewPosting{
			EntityID: entityID, SourceID: sourceID, ExternalID: raw.ExternalID,
			Title: post.Title, URL: post.URL, Locations: post.Locations, Category: post.Category,
			IsSummer:  isSummer,
			FirstSeen: raw.ObservedAt, LastSeen: raw.ObservedAt,
			PayMin: payMin, PayMax: payMax, PayPeriod: post.PayPeriod, PayCurrency: post.PayCurrency,
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

	// Stage the event in the outbox inside the same tx as its DB writes, so a
	// committed event can never be lost relative to its NATS publish (the
	// transactional-outbox pattern; alert-loss-trio H2).
	var outboxID int64
	var subject string
	var payload []byte
	if emit != nil {
		subject = bus.EventsPrefix + emit.Type
		payload, _ = json.Marshal(*emit)
		outboxID, err = p.store.InsertOutbox(ctx, tx, subject, payload)
		if err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Happy path: publish inline so latency stays sub-second. A failure here is
	// non-fatal — the row stays unpublished and the relay (RelayOutbox) resends.
	if emit != nil {
		p.publishStaged(ctx, outboxID, subject, payload)
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

// publishStaged sends a staged outbox event and, on success, marks it published
// so the relay won't resend it. A failure is non-fatal: the row stays unpublished
// and RelayOutbox retries. This is at-least-once: if a publish succeeds but the
// row isn't marked — a process crash, or the relay sweep landing in the small gap
// between Publish and the mark — the relay re-sends it, so at most one duplicate.
// The alert path tolerates that (and the dispatcher's 48h freshness gate bounds
// it). A created_at grace window on the relay query would shrink this to
// crash-only, but the duplicate is rare and harmless, so it's not worth the
// complexity at single-user volume.
func (p *Processor) publishStaged(ctx context.Context, id int64, subject string, payload []byte) {
	if err := p.bus.Publish(subject, payload); err != nil {
		p.log.Warn("inline publish failed; relay will retry", "subject", subject, "outbox_id", id, "err", err)
		return
	}
	if err := p.store.MarkOutboxPublished(ctx, p.store.Pool, id); err != nil {
		p.log.Error("mark outbox published", "outbox_id", id, "err", err)
	}
}

// RelayOutbox publishes staged events that a prior inline publish failed to send
// — the transactional-outbox backstop. Inline publish (publishStaged) is the
// happy path that keeps latency sub-second; this sweep guarantees a NATS hiccup
// at commit time never permanently drops an alert. Returns the count republished.
// (Observability for now is a log line per non-empty sweep; a Prometheus gauge on
// the unpublished backlog is the upgrade once metrics infra lands — DESIGN §7.)
func (p *Processor) RelayOutbox(ctx context.Context, limit int) (int, error) {
	rows, err := p.store.UnpublishedOutbox(ctx, limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if err := p.bus.Publish(r.Subject, r.Payload); err != nil {
			p.log.Warn("relay publish failed; will retry next sweep", "subject", r.Subject, "outbox_id", r.ID, "err", err)
			continue
		}
		if err := p.store.MarkOutboxPublished(ctx, p.store.Pool, r.ID); err != nil {
			p.log.Error("relay mark published", "outbox_id", r.ID, "err", err)
			continue
		}
		n++
	}
	if n > 0 {
		p.log.Info("relayed staged events", "count", n)
	}
	return n, nil
}

// recordDecision records the resolution decision for a raw company string. The
// in-process p.recorded set is only a hot-path optimization — it skips a redundant
// DB write for strings already handled this run (the feed re-emits everything each
// poll). The real, cross-restart guarantee of one decision per string is the
// unique index on resolution_decisions.raw_text (RecordResolution does ON CONFLICT
// DO NOTHING), so a restart no longer re-appends rows for the whole feed.
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
