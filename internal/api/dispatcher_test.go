package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Mekski/reqradar/internal/signal"
)

func TestWithinFreshness(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		eventTime time.Time
		want      bool
	}{
		{"just posted", now, true},
		{"47h old — fresh", now.Add(-47 * time.Hour), true},
		{"exactly 48h — still fresh (<=)", now.Add(-alertFreshness), true},
		{"49h old — stale", now.Add(-49 * time.Hour), false},
		{"years old (backfill) — stale", now.Add(-3 * 365 * 24 * time.Hour), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withinFreshness(c.eventTime, now); got != c.want {
				t.Errorf("withinFreshness(%v) = %v, want %v", c.eventTime, got, c.want)
			}
		})
	}
}

func TestShouldAlert(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		cfg  string
		want bool
	}{
		{"default: posting_opened alerts", "posting_opened", "", true},
		{"default: jd_changed alerts", "jd_changed", "", true},
		{"default: unknown type does not", "posting_closed", "", false},
		{"empty config object falls back to default", "posting_opened", "{}", true},
		{"custom list includes type", "jd_changed", `{"event_types":["jd_changed"]}`, true},
		{"custom list excludes type", "posting_opened", `{"event_types":["jd_changed"]}`, false},
		{"empty custom list falls back to default", "posting_opened", `{"event_types":[]}`, true},
		{"malformed config falls back to default", "posting_opened", `{not json`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var cfg json.RawMessage
			if c.cfg != "" {
				cfg = json.RawMessage(c.cfg)
			}
			if got := shouldAlert(c.typ, cfg); got != c.want {
				t.Errorf("shouldAlert(%q, %s) = %v, want %v", c.typ, c.cfg, got, c.want)
			}
		})
	}
}

func eventData(t *testing.T, m map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	return b
}

func TestFormatAlert(t *testing.T) {
	t.Run("posting_opened with all fields", func(t *testing.T) {
		e := signal.Event{Type: "posting_opened", Data: eventData(t, map[string]any{
			"company": "Anthropic", "title": "SWE Intern", "url": "https://x/1",
			"locations": []string{"SF", "NYC"},
		})}
		got := formatAlert(e)
		for _, want := range []string{"🔔", "Anthropic", "posted", "SWE Intern", "📍 SF, NYC", "🔗 https://x/1"} {
			if !strings.Contains(got, want) {
				t.Errorf("formatAlert missing %q in:\n%s", want, got)
			}
		}
	})

	t.Run("jd_changed uses the update verb", func(t *testing.T) {
		e := signal.Event{Type: "jd_changed", Data: eventData(t, map[string]any{
			"company": "Anthropic", "title": "SWE Intern",
		})}
		got := formatAlert(e)
		if !strings.Contains(got, "✏️") || !strings.Contains(got, "updated a posting") {
			t.Errorf("jd_changed format = %q, want update verb", got)
		}
	})

	t.Run("omits location and url lines when absent", func(t *testing.T) {
		e := signal.Event{Type: "posting_opened", Data: eventData(t, map[string]any{
			"company": "Anthropic", "title": "SWE Intern",
		})}
		got := formatAlert(e)
		if strings.Contains(got, "📍") || strings.Contains(got, "🔗") {
			t.Errorf("expected no location/url lines, got:\n%s", got)
		}
	})
}

func TestFormatFirehose(t *testing.T) {
	e := signal.Event{Type: "firehose", Data: eventData(t, map[string]any{
		"company": "Stripe", "title": "Backend Intern", "url": "https://x/2", "category": "Software Engineering",
	})}
	got := formatFirehose(e)
	for _, want := range []string{"🆕", "Stripe", "Backend Intern", "📂 Software Engineering", "🔗 https://x/2"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatFirehose missing %q in:\n%s", want, got)
		}
	}

	// Category/url lines are omitted when empty.
	bare := signal.Event{Type: "firehose", Data: eventData(t, map[string]any{
		"company": "Stripe", "title": "Backend Intern",
	})}
	if got := formatFirehose(bare); strings.Contains(got, "📂") || strings.Contains(got, "🔗") {
		t.Errorf("expected no category/url lines, got:\n%s", got)
	}
}
