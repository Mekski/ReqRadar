package greenhouse

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

// readFixture loads the captured real Greenhouse board sample (see testdata):
// one real intern ("PhD Intern", dept "Early Career Internships"), one
// false-positive ("Sr Internal Auditor" — "Internal" must NOT match), and one
// full-time role (dept "Early Career Full-Time").
func readFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/board_sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// TestParseBoardGolden asserts the live wire format parses into the fields we
// depend on. A Greenhouse API change (renamed/retyped field) fails here, not
// silently — the "format drift must fail CI" guarantee (DESIGN §7).
func TestParseBoardGolden(t *testing.T) {
	b, err := parseBoard(readFixture(t))
	if err != nil {
		t.Fatalf("parseBoard: %v", err)
	}
	if len(b.Jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(b.Jobs))
	}
	j := b.Jobs[0]
	if j.ID != 7142298 {
		t.Errorf("id = %d", j.ID)
	}
	if j.CompanyName != "Roblox" {
		t.Errorf("company_name = %q", j.CompanyName)
	}
	if j.Location.Name != "San Mateo, CA, United States" {
		t.Errorf("location = %q", j.Location.Name)
	}
	if len(j.Departments) != 1 || j.Departments[0].Name != "Early Career Internships" {
		t.Errorf("departments = %+v", j.Departments)
	}
	if j.FirstPublished != "2025-10-14T16:41:06-04:00" {
		t.Errorf("first_published = %q", j.FirstPublished)
	}
}

func TestParseBoardBadJSON(t *testing.T) {
	if _, err := parseBoard([]byte("{not json")); err == nil {
		t.Error("expected error on malformed body, got nil")
	}
}

// TestIsInternship is the load-bearing filter: word-boundary matching so we keep
// real interns (by title or department) but reject "Internal"/"International",
// which are real false positives seen on live boards.
func TestIsInternship(t *testing.T) {
	cases := []struct {
		name  string
		title string
		dept  string
		want  bool
	}{
		{"title intern", "Software Engineer Intern, Summer 2027", "Engineering", true},
		{"title internship", "Backend Internship", "", true},
		{"title interns plural", "Research Interns", "", true},
		{"dept signals intern", "[2026] Applied Scientist - PhD Intern", "Early Career Internships", true},
		{"dept only", "Applied Scientist", "Early Career Internships", true},
		{"internal is not intern", "Sr Internal Auditor", "Finance", false},
		{"international is not intern", "Account Exec - International", "Sales", false},
		{"full-time role", "[2026] Data Scientist, Social", "Early Career Full-Time", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := job{Title: c.title}
			if c.dept != "" {
				j.Departments = []struct {
					Name string `json:"name"`
				}{{Name: c.dept}}
			}
			if got := isInternship(j); got != c.want {
				t.Errorf("isInternship(%q / %q) = %v, want %v", c.title, c.dept, got, c.want)
			}
		})
	}
}

// TestContentHashIgnoresVolatileFields: the job-watch lesson (DESIGN §1). The hash
// excludes the content HTML and updated_at (Greenhouse rewrites body markup), so an
// unchanged req re-emits identically; JD-meaningful fields each change it.
func TestContentHashIgnoresVolatileFields(t *testing.T) {
	base := job{ID: 1, Title: "SWE Intern", CompanyName: "Roblox", AbsoluteURL: "https://x/1"}
	base.Location.Name = "SF"
	base.Departments = []struct {
		Name string `json:"name"`
	}{{Name: "Early Career Internships"}}
	h := contentHash("roblox", base)

	// Volatile churn must NOT change the hash.
	volatile := base
	volatile.Content = "<div>rewritten markup</div>"
	volatile.UpdatedAt = "2026-06-15T00:00:00-04:00"
	volatile.FirstPublished = "2099-01-01T00:00:00-04:00"
	if got := contentHash("roblox", volatile); got != h {
		t.Errorf("volatile-only change altered hash:\n base = %s\n got  = %s", h, got)
	}

	// Each JD-meaningful field MUST change the hash.
	if contentHash("xai", base) == h {
		t.Error("changing org did not change the hash")
	}
	t2 := base
	t2.Title = "Senior SWE Intern"
	if contentHash("roblox", t2) == h {
		t.Error("changing title did not change the hash")
	}
	t3 := base
	t3.Location.Name = "NYC"
	if contentHash("roblox", t3) == h {
		t.Error("changing location did not change the hash")
	}
}

func TestToSignal(t *testing.T) {
	observedAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	j := job{ID: 7142298, Title: "PhD Intern", CompanyName: "Roblox", AbsoluteURL: "https://x/1", FirstPublished: "2025-10-14T16:41:06-04:00"}
	j.Location.Name = "San Mateo, CA"
	sig := toSignal("greenhouse", "roblox", j, observedAt)

	if sig.Source != "greenhouse" {
		t.Errorf("source = %q", sig.Source)
	}
	if sig.ExternalID != "7142298" {
		t.Errorf("external_id = %q, want the stringified job id", sig.ExternalID)
	}
	if sig.Kind != "posting" {
		t.Errorf("kind = %q", sig.Kind)
	}
	// EventTime is the req's first_published, parsed to UTC; never ObservedAt.
	wantEvent := time.Date(2025, 10, 14, 20, 41, 6, 0, time.UTC) // -04:00 -> UTC
	if !sig.EventTime.Equal(wantEvent) {
		t.Errorf("event_time = %v, want %v", sig.EventTime, wantEvent)
	}
	if !sig.ObservedAt.Equal(observedAt) {
		t.Errorf("observed_at = %v, want %v", sig.ObservedAt, observedAt)
	}

	// Payload carries the org slug + the fields the processor needs.
	var e emitted
	if err := json.Unmarshal(sig.Payload, &e); err != nil {
		t.Fatalf("payload not valid json: %v", err)
	}
	if e.Org != "roblox" || e.CompanyName != "Roblox" || e.URL != "https://x/1" {
		t.Errorf("payload = %+v", e)
	}
	if !reflect.DeepEqual(e.Locations, []string{"San Mateo, CA"}) {
		t.Errorf("locations = %v", e.Locations)
	}
}

// TestEventTimeFallback: a missing/odd first_published must not drop a signal.
func TestEventTimeFallback(t *testing.T) {
	fb := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if got := eventTime("", fb); !got.Equal(fb) {
		t.Errorf("eventTime(\"\") = %v, want fallback %v", got, fb)
	}
}

// rewriteTransport redirects every request to the test server regardless of the
// hardcoded boards-api.greenhouse.io host (same approach as the simplify tests).
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

	c := newTestCollector(t, ts, "roblox")
	sigs, err := c.Collect(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// Fixture has 1 real intern + 1 "Internal" false-positive + 1 full-time;
	// only the intern is emitted.
	if len(sigs) != 1 {
		t.Fatalf("got %d signals, want 1 (interns only)", len(sigs))
	}
	if sigs[0].ExternalID != "7142298" {
		t.Errorf("emitted %q, want the PhD Intern (7142298)", sigs[0].ExternalID)
	}
}

// TestCollectConditionalGET verifies per-org ETag flow: first poll captures the
// ETag, second sends If-None-Match and a 304 yields no signals.
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

	c := newTestCollector(t, ts, "roblox")
	if _, err := c.Collect(context.Background(), time.Time{}); err != nil {
		t.Fatalf("first Collect: %v", err)
	}
	if sawINM != "" {
		t.Errorf("first request sent If-None-Match=%q, want none", sawINM)
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

// TestCollectPartialFailure: one org returning 500 must not drop the other org's
// signals (only an all-orgs failure surfaces an error to the runner).
func TestCollectPartialFailure(t *testing.T) {
	fixture := readFixture(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/boards/broken/jobs" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write(fixture)
	}))
	defer ts.Close()

	c := newTestCollector(t, ts, "broken", "roblox")
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
