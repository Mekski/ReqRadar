package ashby

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

// readFixture loads the captured real Ashby board sample (see testdata): one
// intern (employmentType "Intern"), one full-time SWE, one contract role.
func readFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/board_sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// TestParseBoardGolden asserts the live wire format parses into the fields we use.
// An Ashby API change fails here, not silently (DESIGN §7 format-drift guard).
func TestParseBoardGolden(t *testing.T) {
	b, err := parseBoard(readFixture(t))
	if err != nil {
		t.Fatalf("parseBoard: %v", err)
	}
	if len(b.Jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(b.Jobs))
	}
	j := b.Jobs[0]
	if j.Title != "Software Engineer Intern (Fall 2026)" {
		t.Errorf("title = %q", j.Title)
	}
	if j.EmploymentType != "Intern" {
		t.Errorf("employmentType = %q", j.EmploymentType)
	}
	if !j.IsListed {
		t.Errorf("isListed = %v, want true", j.IsListed)
	}
	if j.JobURL == "" {
		t.Errorf("jobUrl is empty")
	}
}

func TestParseBoardBadJSON(t *testing.T) {
	if _, err := parseBoard([]byte("not json")); err == nil {
		t.Error("expected error on malformed body, got nil")
	}
}

// TestIsInternship: the filter is exact on the structured employmentType, so
// "FullTime"/"Contract" are rejected without any title heuristic.
func TestIsInternship(t *testing.T) {
	cases := []struct {
		empType string
		want    bool
	}{
		{"Intern", true},
		{"Internship", true},
		{"FullTime", false},
		{"Contract", false},
		{"PartTime", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isInternship(job{EmploymentType: c.empType}); got != c.want {
			t.Errorf("isInternship(%q) = %v, want %v", c.empType, got, c.want)
		}
	}
}

func TestContentHashIgnoresVolatileDescription(t *testing.T) {
	base := job{ID: "u1", Title: "SWE Intern", JobURL: "https://x/1", Location: "SF", Department: "Eng", EmploymentType: "Intern"}
	h := contentHash("notion", base)

	volatile := base
	volatile.DescriptionHTML = "<p>rewritten</p>"
	volatile.PublishedAt = "2099-01-01T00:00:00.000+00:00"
	volatile.IsListed = false
	if got := contentHash("notion", volatile); got != h {
		t.Errorf("volatile-only change altered hash")
	}

	if contentHash("openai", base) == h {
		t.Error("changing org did not change the hash")
	}
	t2 := base
	t2.Title = "Senior SWE Intern"
	if contentHash("notion", t2) == h {
		t.Error("changing title did not change the hash")
	}
}

func TestToSignal(t *testing.T) {
	observedAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	j := job{ID: "uuid-1", Title: "SWE Intern", JobURL: "https://jobs.ashbyhq.com/notion/uuid-1", Location: "San Francisco, California", PublishedAt: "2026-04-06T22:40:58.312+00:00"}
	sig := toSignal("ashby", "notion", j, observedAt)

	if sig.Source != "ashby" {
		t.Errorf("source = %q", sig.Source)
	}
	if sig.ExternalID != "uuid-1" {
		t.Errorf("external_id = %q", sig.ExternalID)
	}
	if sig.Kind != "posting" {
		t.Errorf("kind = %q", sig.Kind)
	}
	wantEvent := time.Date(2026, 4, 6, 22, 40, 58, 312000000, time.UTC)
	if !sig.EventTime.Equal(wantEvent) {
		t.Errorf("event_time = %v, want %v", sig.EventTime, wantEvent)
	}
	var e emitted
	if err := json.Unmarshal(sig.Payload, &e); err != nil {
		t.Fatalf("payload not valid json: %v", err)
	}
	if e.Org != "notion" || e.URL != "https://jobs.ashbyhq.com/notion/uuid-1" || e.Location != "San Francisco, California" {
		t.Errorf("payload = %+v", e)
	}
}

type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func newTestCollector(t *testing.T, server *httptest.Server, orgs ...string) *Collector {
	t.Helper()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	return &Collector{
		client: &http.Client{Transport: rewriteTransport{base}},
		orgsFn: func(context.Context) ([]string, error) { return orgs, nil },
		etags:  map[string]string{},
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestCollectInternsOnly(t *testing.T) {
	fixture := readFixture(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.Write(fixture)
	}))
	defer ts.Close()

	c := newTestCollector(t, ts, "notion")
	sigs, err := c.Collect(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// Fixture: 1 Intern + 1 FullTime + 1 Contract; only the intern is emitted.
	if len(sigs) != 1 {
		t.Fatalf("got %d signals, want 1 (interns only)", len(sigs))
	}
}

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

	c := newTestCollector(t, ts, "notion")
	if _, err := c.Collect(context.Background(), time.Time{}); err != nil {
		t.Fatalf("first Collect: %v", err)
	}
	sigs, err := c.Collect(context.Background(), time.Time{})
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

func TestCollectPartialFailure(t *testing.T) {
	fixture := readFixture(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/posting-api/job-board/broken" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(fixture)
	}))
	defer ts.Close()

	c := newTestCollector(t, ts, "broken", "notion")
	sigs, err := c.Collect(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("partial failure should not error the run: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("got %d signals, want 1 from the healthy org", len(sigs))
	}
}

func TestCollectAllFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := newTestCollector(t, ts, "a", "b")
	if _, err := c.Collect(context.Background(), time.Time{}); err == nil {
		t.Error("expected error when every org fails, got nil")
	}
}
