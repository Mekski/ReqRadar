// Package api hosts the dashboard REST server and the Telegram alert dispatcher.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
	"github.com/Mekski/reqradar/internal/telegram"
)

type Dispatcher struct {
	store *store.Store
	tg    *telegram.Client
	log   *slog.Logger
}

func NewDispatcher(st *store.Store, tg *telegram.Client, log *slog.Logger) *Dispatcher {
	return &Dispatcher{store: st, tg: tg, log: log}
}

// Handle dispatches one event. Firehose events (broad, non-watchlist) go to all
// users; watchlist events go to that entity's watchers with latency recorded.
// Events reaching here are already new (the consumer uses DeliverNew).
// alertFreshness gates alerts to roles actually posted recently. Events are
// always STORED (timing/history), but we only Telegram-alert on fresh ones — so
// newly-tracking a company or running a backfill imports its existing roles
// silently instead of flooding. Matches job-watch's "only new rows" behavior.
const alertFreshness = 48 * time.Hour

func (d *Dispatcher) Handle(ctx context.Context, e signal.Event) error {
	if time.Since(e.EventTime) > alertFreshness {
		return nil // role wasn't posted recently — stored, but not alert-worthy
	}
	if e.Type == "firehose" {
		return d.handleFirehose(ctx, e)
	}
	return d.handleWatchlist(ctx, e)
}

// handleFirehose sends a lightweight "new internship" alert to every user.
func (d *Dispatcher) handleFirehose(ctx context.Context, e signal.Event) error {
	chatIDs, err := d.store.AllUserChatIDs(ctx)
	if err != nil {
		return fmt.Errorf("chat ids: %w", err)
	}
	text := formatFirehose(e)
	for _, chatID := range chatIDs {
		if err := d.tg.SendMessage(ctx, chatID, text); err != nil {
			d.log.Error("firehose send", "err", err)
		}
	}
	d.log.Info("firehose alert sent", "recipients", len(chatIDs))
	return nil
}

func (d *Dispatcher) handleWatchlist(ctx context.Context, e signal.Event) error {
	watchers, err := d.store.UsersWatchingEntity(ctx, e.EntityID)
	if err != nil {
		return fmt.Errorf("watchers: %w", err)
	}
	text := formatAlert(e)
	for _, w := range watchers {
		if !shouldAlert(e.Type, w.AlertConfig) {
			continue
		}
		if err := d.tg.SendMessage(ctx, w.ChatID, text); err != nil {
			d.log.Error("telegram send", "user", w.UserID, "err", err)
			continue
		}
		ms := int(time.Since(e.ObservedAt).Milliseconds())
		if ms < 0 {
			ms = 0
		}
		if err := d.store.InsertAlert(ctx, w.UserID, e.EventID, ms); err != nil {
			d.log.Error("record alert", "user", w.UserID, "err", err)
		}
		d.log.Info("alert sent", "entity", e.EntityID, "type", e.Type, "detect_to_alert_ms", ms)
	}
	return nil
}

var defaultAlertTypes = map[string]bool{"posting_opened": true, "jd_changed": true}

// shouldAlert honors an optional {"event_types": [...]} override in alert_config,
// else falls back to the default set.
func shouldAlert(typ string, cfg json.RawMessage) bool {
	var c struct {
		EventTypes []string `json:"event_types"`
	}
	if len(cfg) > 0 {
		_ = json.Unmarshal(cfg, &c)
	}
	if len(c.EventTypes) == 0 {
		return defaultAlertTypes[typ]
	}
	for _, t := range c.EventTypes {
		if t == typ {
			return true
		}
	}
	return false
}

func formatAlert(e signal.Event) string {
	var d struct {
		Company   string   `json:"company"`
		Title     string   `json:"title"`
		URL       string   `json:"url"`
		Locations []string `json:"locations"`
	}
	_ = json.Unmarshal(e.Data, &d)

	emoji, verb := "🔔", "posted"
	if e.Type == "jd_changed" {
		emoji, verb = "✏️", "updated a posting"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s: %s", emoji, d.Company, verb, d.Title)
	if loc := strings.Join(d.Locations, ", "); loc != "" {
		fmt.Fprintf(&b, "\n📍 %s", loc)
	}
	if d.URL != "" {
		fmt.Fprintf(&b, "\n🔗 %s", d.URL)
	}
	return b.String()
}

// formatFirehose renders a broad (non-watchlist) alert. The 🆕 marker
// distinguishes it from a 🔔 watchlist alert at a glance.
func formatFirehose(e signal.Event) string {
	var d struct {
		Company  string `json:"company"`
		Title    string `json:"title"`
		URL      string `json:"url"`
		Category string `json:"category"`
	}
	_ = json.Unmarshal(e.Data, &d)

	var b strings.Builder
	fmt.Fprintf(&b, "🆕 %s — %s", d.Company, d.Title)
	if d.Category != "" {
		fmt.Fprintf(&b, "\n📂 %s", d.Category)
	}
	if d.URL != "" {
		fmt.Fprintf(&b, "\n🔗 %s", d.URL)
	}
	return b.String()
}
