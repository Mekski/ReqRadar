// Package simplify implements the simplify-listings collector: it polls the
// SimplifyJobs listings.json and emits one RawSignal per active posting. It does
// not interpret or dedupe — that is the processor's job — so it emits all active
// listings whenever the file changes and lets the processor dedupe by content
// hash. A conditional GET (ETag) avoids re-emitting when the file is unchanged.
package simplify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Mekski/reqradar/internal/collector"
	"github.com/Mekski/reqradar/internal/signal"
)

type config struct {
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	ListingsPath string `json:"listings_path"`
}

type Collector struct {
	client *http.Client
	url    string
	etag   string // in-memory conditional-GET state; a restart re-fetches once
	log    *slog.Logger
}

// New is the collector.Factory for simplify-listings.
func New(raw json.RawMessage, log *slog.Logger) (collector.Collector, error) {
	var cfg config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Owner == "" || cfg.Repo == "" || cfg.ListingsPath == "" {
		return nil, fmt.Errorf("owner, repo, and listings_path are required")
	}
	return &Collector{
		client: &http.Client{Timeout: 30 * time.Second},
		// listings.json lives on the dev branch.
		url: fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/dev/%s", cfg.Owner, cfg.Repo, cfg.ListingsPath),
		log: log,
	}, nil
}

func (c *Collector) Name() string            { return "simplify-listings" }
func (c *Collector) Schedule() time.Duration { return 5 * time.Minute }

// listing mirrors one entry in listings.json. Field types verified against the
// live file 2026-06-13: bools, epoch-second ints, string arrays.
type listing struct {
	ID          string   `json:"id"`
	CompanyName string   `json:"company_name"`
	Title       string   `json:"title"`
	Locations   []string `json:"locations"`
	Terms       []string `json:"terms"`
	Category    string   `json:"category"`
	URL         string   `json:"url"`
	Sponsorship string   `json:"sponsorship"`
	Degrees     []string `json:"degrees"`
	Active      bool     `json:"active"`
	DatePosted  int64    `json:"date_posted"`
	DateUpdated int64    `json:"date_updated"`
}

func (c *Collector) Collect(ctx context.Context, _ time.Time) ([]signal.RawSignal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, err
	}
	if c.etag != "" {
		req.Header.Set("If-None-Match", c.etag)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, nil // file unchanged since last poll
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var listings []listing
	if err := json.Unmarshal(body, &listings); err != nil {
		return nil, fmt.Errorf("parse listings: %w", err)
	}
	c.etag = resp.Header.Get("ETag")

	now := time.Now()
	var signals []signal.RawSignal
	for i := range listings {
		l := listings[i]
		if !l.Active {
			continue // polling emits only currently-open postings; history is backfill's job
		}
		payload, err := json.Marshal(l)
		if err != nil {
			c.log.Error("marshal listing", "id", l.ID, "err", err)
			continue
		}
		signals = append(signals, signal.RawSignal{
			Source:      c.Name(),
			ExternalID:  l.ID,
			Kind:        signal.KindPosting,
			EventTime:   time.Unix(l.DatePosted, 0).UTC(),
			ObservedAt:  now,
			Payload:     payload,
			ContentHash: contentHash(l),
		})
	}
	return signals, nil
}

// contentHash hashes only the JD-meaningful fields, deliberately excluding
// volatile ones (date_updated, id, source) so a re-emit of an unchanged posting
// produces an identical hash and the processor dedupes it. This is the
// canonicalize-before-hashing rule inherited from job-watch (DESIGN §1 Lineage).
func contentHash(l listing) string {
	canonical := struct {
		Company     string   `json:"company"`
		Title       string   `json:"title"`
		Locations   []string `json:"locations"`
		Terms       []string `json:"terms"`
		Category    string   `json:"category"`
		URL         string   `json:"url"`
		Sponsorship string   `json:"sponsorship"`
		Degrees     []string `json:"degrees"`
		Active      bool     `json:"active"`
	}{l.CompanyName, l.Title, l.Locations, l.Terms, l.Category, l.URL, l.Sponsorship, l.Degrees, l.Active}
	b, _ := json.Marshal(canonical) // Go marshals struct fields in declared order — deterministic
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
