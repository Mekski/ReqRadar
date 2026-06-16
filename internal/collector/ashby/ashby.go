// Package ashby implements the ashby collector: it polls the public Ashby
// posting-api job board for each configured org and emits one RawSignal per
// *internship* posting. One collector handles all orgs in the source config; its
// Name() is the shared "ashby" source.
//
// Ashby gives a structured employmentType ("Intern" | "FullTime" | ...), so —
// unlike Greenhouse, which needs a word-boundary title/department heuristic — the
// intern filter here is exact (employmentType == Intern). Boards still carry the
// whole company req list, so filtering before emit is a volume guard (like
// simplify's active-only filter); the real normalization stays in the processor.
//
// No Backfiller: the API exposes only currently-listed reqs, so Ashby contributes
// live detection + pay (in the JD HTML), not history. Ashby exposes no company
// name, so the processor resolves via the org slug (seeded as an alias).
package ashby

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Mekski/reqradar/internal/collector"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
)

type Collector struct {
	client *http.Client
	// orgsFn returns the board slugs to poll, read from the watchlist
	// (entities.metadata.ats) each cycle so a runtime-discovered slug is picked up
	// next poll with no restart. Injectable for testing.
	orgsFn func(context.Context) ([]string, error)
	etags  map[string]string // per-org conditional-GET state; a restart re-fetches once
	log    *slog.Logger
}

// New is the collector.Factory for ashby. The poll-list comes from the watchlist
// via the store each cycle, not the static config.
func New(_ json.RawMessage, st *store.Store, log *slog.Logger) (collector.Collector, error) {
	return &Collector{
		client: &http.Client{Timeout: 30 * time.Second},
		orgsFn: func(ctx context.Context) ([]string, error) { return st.ATSOrgs(ctx, "ashby") },
		etags:  map[string]string{},
		log:    log,
	}, nil
}

func (c *Collector) Name() string            { return "ashby" }
func (c *Collector) Schedule() time.Duration { return 10 * time.Minute }

// job mirrors the fields we use from an Ashby posting. Verified against the live
// API 2026-06-15 (api.ashbyhq.com/posting-api/job-board/<org>?includeCompensation=true).
type job struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	EmploymentType  string `json:"employmentType"`
	Department      string `json:"department"`
	Team            string `json:"team"`
	Location        string `json:"location"`
	IsListed        bool   `json:"isListed"`
	PublishedAt     string `json:"publishedAt"`
	JobURL          string `json:"jobUrl"`
	DescriptionHTML string `json:"descriptionHtml"`
}

type board struct {
	Jobs []job `json:"jobs"`
}

// emitted is the wire payload the collector publishes: the fields the processor
// needs plus the org slug we queried (resolution uses it — Ashby has no company
// name). The wire format is the contract; the processor keeps its own copy and
// golden-file tests guard drift.
type emitted struct {
	Org         string `json:"org"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Location    string `json:"location"`
	Department  string `json:"department"`
	Description string `json:"description"`
	PublishedAt string `json:"published_at"`
}

func isInternship(j job) bool {
	return strings.Contains(strings.ToLower(j.EmploymentType), "intern")
}

func (c *Collector) Collect(ctx context.Context, _ time.Time) ([]signal.RawSignal, error) {
	orgs, err := c.orgsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("load ashby orgs: %w", err)
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
			if !jobs[i].IsListed || !isInternship(jobs[i]) {
				continue
			}
			signals = append(signals, toSignal(c.Name(), org, jobs[i], now))
		}
	}

	// One org failing must not drop the others' signals; only surface an error if
	// every org failed (the runner records a run error as count=0).
	if ok == 0 && firstErr != nil {
		return nil, firstErr
	}
	return signals, nil
}

func (c *Collector) fetchBoard(ctx context.Context, org string) (jobs []job, notModified bool, err error) {
	url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s?includeCompensation=true", org)
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

// toSignal builds a RawSignal from an Ashby posting. EventTime is publishedAt
// (when the req went live); ObservedAt is now (t0 for latency).
func toSignal(source, org string, j job, observedAt time.Time) signal.RawSignal {
	payload, _ := json.Marshal(emitted{
		Org: org, ID: j.ID, Title: j.Title, URL: j.JobURL,
		Location: j.Location, Department: j.Department,
		Description: j.DescriptionHTML, PublishedAt: j.PublishedAt,
	})
	return signal.RawSignal{
		Source:      source,
		ExternalID:  j.ID,
		Kind:        signal.KindPosting,
		EventTime:   eventTime(j.PublishedAt, observedAt),
		ObservedAt:  observedAt,
		Payload:     payload,
		ContentHash: contentHash(org, j),
	}
}

// eventTime parses Ashby's RFC3339 timestamps (with fractional seconds); on
// failure it falls back to observedAt so a missing date never drops a signal.
func eventTime(s string, fallback time.Time) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return fallback
}

// contentHash hashes only stable, JD-meaningful fields. It excludes the
// description HTML (volatile markup → phantom jd_changed re-alerts; the job-watch
// lesson, DESIGN §1). Pay is read fresh from the description at normalize time.
func contentHash(org string, j job) string {
	canonical := struct {
		Org        string `json:"org"`
		Title      string `json:"title"`
		URL        string `json:"url"`
		Location   string `json:"location"`
		Department string `json:"department"`
		Employment string `json:"employment"`
	}{org, j.Title, j.JobURL, j.Location, j.Department, j.EmploymentType}
	b, _ := json.Marshal(canonical) // declared-order marshal is deterministic
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
