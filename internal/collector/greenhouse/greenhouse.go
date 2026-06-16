// Package greenhouse implements the greenhouse collector: it polls the public
// Greenhouse job-board API (boards-api.greenhouse.io) for each configured org and
// emits one RawSignal per *internship* posting. One collector handles all orgs in
// the source config; its Name() is the shared "greenhouse" source.
//
// Greenhouse boards carry the company's WHOLE req list (full-time included), so —
// like simplify's active-only filter — this collector applies one coarse,
// operational filter before emitting: it keeps only internship reqs (word-boundary
// "intern"/"internship" in the title or department). Without it, a poll would push
// hundreds of full-time roles per org into NATS, every one of which resolves to a
// watchlist company and would wrongly fire posting_opened alerts. The authoritative
// normalization (title -> terms/category, company resolution) still happens in the
// processor; this stays a volume guard, not semantic interpretation.
//
// No Backfiller: the API only exposes currently-open reqs, so Greenhouse
// contributes live detection + pay (in the JD content), not history.
package greenhouse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/Mekski/reqradar/internal/collector"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
)

type Collector struct {
	client *http.Client
	// orgsFn returns the board slugs to poll. In prod it reads them from the
	// watchlist (entities.metadata.ats) each cycle, so a slug discovered at runtime
	// is picked up on the next poll with no restart. Injectable for testing.
	orgsFn func(context.Context) ([]string, error)
	etags  map[string]string // per-org conditional-GET state; a restart re-fetches once
	log    *slog.Logger
}

// New is the collector.Factory for greenhouse. The poll-list is no longer in the
// static config — it comes from the watchlist via the store each cycle.
func New(_ json.RawMessage, st *store.Store, log *slog.Logger) (collector.Collector, error) {
	return &Collector{
		client: &http.Client{Timeout: 30 * time.Second},
		orgsFn: func(ctx context.Context) ([]string, error) { return st.ATSOrgs(ctx, "greenhouse") },
		etags:  map[string]string{},
		log:    log,
	}, nil
}

func (c *Collector) Name() string            { return "greenhouse" }
func (c *Collector) Schedule() time.Duration { return 10 * time.Minute }

// job mirrors the fields we use from a Greenhouse board entry. Verified against
// the live API 2026-06-15 (boards-api.greenhouse.io/v1/boards/<org>/jobs?content=true).
type job struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	CompanyName    string `json:"company_name"`
	AbsoluteURL    string `json:"absolute_url"`
	FirstPublished string `json:"first_published"`
	UpdatedAt      string `json:"updated_at"`
	Content        string `json:"content"`
	Location       struct {
		Name string `json:"name"`
	} `json:"location"`
	Departments []struct {
		Name string `json:"name"`
	} `json:"departments"`
}

type board struct {
	Jobs []job `json:"jobs"`
}

// emitted is the wire payload the collector publishes: the fields the processor
// needs plus the org slug we queried (so resolution works even if company_name is
// blank). The wire format — not this Go type — is the contract; the processor keeps
// its own copy and golden-file tests guard drift.
type emitted struct {
	Org            string   `json:"org"`
	ID             int64    `json:"id"`
	Title          string   `json:"title"`
	CompanyName    string   `json:"company_name"`
	URL            string   `json:"url"`
	Locations      []string `json:"locations"`
	Departments    []string `json:"departments"`
	Content        string   `json:"content"`
	FirstPublished string   `json:"first_published"`
}

// internRE matches "intern"/"interns"/"internship(s)" as whole words so it does
// NOT catch "Internal" or "International" — a real false-positive source observed
// on live boards ("Sr Internal Auditor", "Federal Civilian Sales - International").
var internRE = regexp.MustCompile(`(?i)\bintern(ship)?s?\b`)

func isInternship(j job) bool {
	if internRE.MatchString(j.Title) {
		return true
	}
	for _, d := range j.Departments {
		if internRE.MatchString(d.Name) {
			return true
		}
	}
	return false
}

func (c *Collector) Collect(ctx context.Context, _ time.Time) ([]signal.RawSignal, error) {
	orgs, err := c.orgsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("load greenhouse orgs: %w", err)
	}
	now := time.Now()
	var signals []signal.RawSignal
	var firstErr error
	ok := 0

	for _, org := range orgs {
		jobs, notModified, err := c.fetchBoard(ctx, org)
		if err != nil {
			c.log.Error("fetch board", "org", org, "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ok++
		if notModified {
			continue
		}
		for i := range jobs {
			if !isInternship(jobs[i]) {
				continue
			}
			signals = append(signals, toSignal(c.Name(), org, jobs[i], now))
		}
	}

	// One org failing must not drop the others' signals (the runner records a run
	// error as count=0). Only surface an error if every org failed.
	if ok == 0 && firstErr != nil {
		return nil, firstErr
	}
	return signals, nil
}

func (c *Collector) fetchBoard(ctx context.Context, org string) (jobs []job, notModified bool, err error) {
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", org)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	if e := c.etags[org]; e != "" {
		req.Header.Set("If-None-Match", e)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	b, err := parseBoard(body)
	if err != nil {
		return nil, false, err
	}
	c.etags[org] = resp.Header.Get("ETag")
	return b.Jobs, false, nil
}

func parseBoard(body []byte) (board, error) {
	var b board
	if err := json.Unmarshal(body, &b); err != nil {
		return board{}, fmt.Errorf("parse board: %w", err)
	}
	return b, nil
}

// toSignal builds a RawSignal from a Greenhouse job. EventTime is the req's
// first_published (when it opened); ObservedAt is now (t0 for latency).
func toSignal(source, org string, j job, observedAt time.Time) signal.RawSignal {
	depts := make([]string, len(j.Departments))
	for i, d := range j.Departments {
		depts[i] = d.Name
	}
	var locations []string
	if j.Location.Name != "" {
		locations = []string{j.Location.Name}
	}
	payload, _ := json.Marshal(emitted{
		Org: org, ID: j.ID, Title: j.Title, CompanyName: j.CompanyName,
		URL: j.AbsoluteURL, Locations: locations, Departments: depts,
		Content: j.Content, FirstPublished: j.FirstPublished,
	})
	return signal.RawSignal{
		Source:      source,
		ExternalID:  strconv.FormatInt(j.ID, 10),
		Kind:        signal.KindPosting,
		EventTime:   eventTime(j.FirstPublished, observedAt),
		ObservedAt:  observedAt,
		Payload:     payload,
		ContentHash: contentHash(org, j),
	}
}

// eventTime parses Greenhouse's RFC3339 timestamps; on failure it falls back to
// observedAt so a missing/odd date never drops a signal.
func eventTime(s string, fallback time.Time) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return fallback
}

// contentHash hashes only stable, JD-meaningful fields. It deliberately excludes
// the content HTML and updated_at: Greenhouse rewrites the body markup often, so
// hashing it would produce phantom jd_changed re-alerts (the canonicalize-before-
// hashing lesson from job-watch, DESIGN §1). Pay is read fresh from content at
// normalize time regardless of the hash.
func contentHash(org string, j job) string {
	depts := make([]string, len(j.Departments))
	for i, d := range j.Departments {
		depts[i] = d.Name
	}
	canonical := struct {
		Org         string   `json:"org"`
		Title       string   `json:"title"`
		Company     string   `json:"company"`
		URL         string   `json:"url"`
		Location    string   `json:"location"`
		Departments []string `json:"departments"`
	}{org, j.Title, j.CompanyName, j.AbsoluteURL, j.Location.Name, depts}
	b, _ := json.Marshal(canonical) // declared-order marshal is deterministic
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
