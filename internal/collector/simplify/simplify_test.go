package simplify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"
)

// readFixture loads the captured real listings.json sample (see testdata).
func readFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/listings_sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// TestParseListingsGolden asserts the wire format from a real captured payload
// parses into the expected typed fields. A SimplifyJobs format change (renamed
// or retyped field) becomes a failing test here, not a silent data gap — this is
// the "format drift must fail CI" guarantee from DESIGN §7.
func TestParseListingsGolden(t *testing.T) {
	listings, err := parseListings(readFixture(t))
	if err != nil {
		t.Fatalf("parseListings: %v", err)
	}
	if len(listings) != 3 {
		t.Fatalf("got %d listings, want 3", len(listings))
	}

	// Spot-check the first entry across the field types we depend on
	// (string, []string, bool, epoch int64).
	pega := listings[0]
	if pega.ID != "79b20a13-a49b-496a-8569-ddd93c0c7b84" {
		t.Errorf("id = %q", pega.ID)
	}
	if pega.CompanyName != "Pega" {
		t.Errorf("company = %q", pega.CompanyName)
	}
	if pega.Category != "Software Engineering" {
		t.Errorf("category = %q", pega.Category)
	}
	if !pega.Active {
		t.Errorf("active = %v, want true", pega.Active)
	}
	if pega.DatePosted != 1761004443 {
		t.Errorf("date_posted = %d", pega.DatePosted)
	}
	if !reflect.DeepEqual(pega.Locations, []string{"Waltham, MA"}) {
		t.Errorf("locations = %v", pega.Locations)
	}

	// The third entry is inactive — Collect must filter it (see TestCollect).
	if listings[2].Active {
		t.Errorf("expected third listing inactive, got active")
	}
}

func TestParseListingsBadJSON(t *testing.T) {
	if _, err := parseListings([]byte("{not an array")); err == nil {
		t.Error("expected error on malformed body, got nil")
	}
}

// TestContentHashIgnoresVolatileFields is the load-bearing job-watch lesson
// (DESIGN §1 Lineage): a re-emit of an unchanged posting must hash identically,
// or the processor re-alerts on noise. The hash is computed over JD-meaningful
// fields only — id, date_posted, and date_updated are deliberately excluded.
func TestContentHashIgnoresVolatileFields(t *testing.T) {
	base := listing{
		ID: "abc", CompanyName: "Anthropic", Title: "SWE Intern",
		Locations: []string{"SF"}, Terms: []string{"Summer 2026"},
		Category: "Software Engineering", URL: "https://x/1",
		Sponsorship: "Other", Degrees: []string{"Bachelor's"}, Active: true,
		DatePosted: 1000, DateUpdated: 1000,
	}
	h := contentHash(base)

	// Volatile churn must NOT change the hash.
	volatile := base
	volatile.ID = "different-id"
	volatile.DatePosted = 2000
	volatile.DateUpdated = 9999
	if got := contentHash(volatile); got != h {
		t.Errorf("volatile-only change altered hash:\n base = %s\n got  = %s", h, got)
	}

	// Each JD-meaningful field MUST change the hash.
	meaningful := []struct {
		name   string
		mutate func(*listing)
	}{
		{"title", func(l *listing) { l.Title = "Senior SWE Intern" }},
		{"url", func(l *listing) { l.URL = "https://x/2" }},
		{"company", func(l *listing) { l.CompanyName = "OpenAI" }},
		{"locations", func(l *listing) { l.Locations = []string{"NYC"} }},
		{"category", func(l *listing) { l.Category = "AI/ML/Data" }},
		{"sponsorship", func(l *listing) { l.Sponsorship = "Offers Sponsorship" }},
		{"active", func(l *listing) { l.Active = false }},
	}
	for _, m := range meaningful {
		t.Run(m.name, func(t *testing.T) {
			changed := base
			m.mutate(&changed)
			if contentHash(changed) == h {
				t.Errorf("changing %s did not change the content hash", m.name)
			}
		})
	}
}

func TestToSignal(t *testing.T) {
	observedAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	l := listing{
		ID: "abc", CompanyName: "Anthropic", Title: "SWE Intern",
		Category: "Software Engineering", URL: "https://x/1",
		Active: true, DatePosted: 1761004443,
	}
	sig := toSignal("simplify-listings", l, observedAt)

	if sig.Source != "simplify-listings" {
		t.Errorf("source = %q", sig.Source)
	}
	if sig.ExternalID != "abc" {
		t.Errorf("external_id = %q, want the listing id", sig.ExternalID)
	}
	if sig.Kind != "posting" {
		t.Errorf("kind = %q", sig.Kind)
	}
	// EventTime is the posting's own date_posted; ObservedAt is ingest time. The
	// two must never be conflated (DESIGN §3.1) — assert they are distinct here.
	wantEvent := time.Unix(1761004443, 0).UTC()
	if !sig.EventTime.Equal(wantEvent) {
		t.Errorf("event_time = %v, want %v", sig.EventTime, wantEvent)
	}
	if !sig.ObservedAt.Equal(observedAt) {
		t.Errorf("observed_at = %v, want %v", sig.ObservedAt, observedAt)
	}
	if sig.ContentHash != contentHash(l) {
		t.Errorf("content_hash not computed from the listing")
	}
	// Payload must round-trip back to the same listing.
	var got listing
	if err := json.Unmarshal(sig.Payload, &got); err != nil {
		t.Fatalf("payload not valid json: %v", err)
	}
	if !reflect.DeepEqual(got, l) {
		t.Errorf("payload round-trip = %+v, want %+v", got, l)
	}
}

// rewriteTransport redirects every request to a test server, regardless of the
// hardcoded raw.githubusercontent.com host Collect builds. This exercises the
// real Collect logic (conditional GET, status handling, active filter) against
// httptest without adding a base-URL knob to production code.
type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func newTestCollector(t *testing.T, server *httptest.Server) *Collector {
	t.Helper()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	return &Collector{
		client:       &http.Client{Transport: rewriteTransport{base}},
		owner:        "SimplifyJobs",
		repo:         "Summer2026-Internships",
		listingsPath: ".github/scripts/listings.json",
		log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestCollectActiveOnly(t *testing.T) {
	fixture := readFixture(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.Write(fixture)
	}))
	defer ts.Close()

	c := newTestCollector(t, ts)
	sigs, err := c.Collect(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// Fixture has 2 active + 1 inactive; polling emits active only.
	if len(sigs) != 2 {
		t.Fatalf("got %d signals, want 2 (active only)", len(sigs))
	}
	got := map[string]bool{sigs[0].ExternalID: true, sigs[1].ExternalID: true}
	for _, id := range []string{
		"79b20a13-a49b-496a-8569-ddd93c0c7b84", // Pega, active
		"2154dbb1-0a02-4ce3-8bc5-6fbe782e9d84", // Tencent, active
	} {
		if !got[id] {
			t.Errorf("missing expected active posting %s", id)
		}
	}
	if got["7142bcf1-4e34-4e69-820e-4649670f700c"] {
		t.Error("inactive Autodesk posting should have been filtered out")
	}
}

// TestCollectConditionalGET verifies the ETag flow end to end: the first poll
// stores the ETag, the second sends If-None-Match and a 304 yields no signals
// (no re-emit of unchanged data).
func TestCollectConditionalGET(t *testing.T) {
	fixture := readFixture(t)
	const etag = `"v1"`
	var sawINM string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawINM = r.Header.Get("If-None-Match")
		if sawINM == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Write(fixture)
	}))
	defer ts.Close()

	c := newTestCollector(t, ts)

	// First poll: no validator sent, full body returned, ETag captured.
	sigs, err := c.Collect(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("first Collect: %v", err)
	}
	if sawINM != "" {
		t.Errorf("first request sent If-None-Match=%q, want none", sawINM)
	}
	if len(sigs) != 2 {
		t.Fatalf("first poll got %d signals, want 2", len(sigs))
	}

	// Second poll: validator sent, server replies 304, no signals.
	sigs, err = c.Collect(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("second Collect: %v", err)
	}
	if sawINM != etag {
		t.Errorf("second request sent If-None-Match=%q, want %q", sawINM, etag)
	}
	if sigs != nil {
		t.Errorf("304 should yield nil signals, got %d", len(sigs))
	}
}

func TestCollectBadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := newTestCollector(t, ts)
	if _, err := c.Collect(context.Background(), time.Time{}); err == nil {
		t.Error("expected error on HTTP 500, got nil")
	}
}
