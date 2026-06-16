// Package simplify implements the simplify-listings collector: it polls the
// SimplifyJobs listings.json and emits one RawSignal per active posting. It does
// not interpret or dedupe — that is the processor's job — so it emits all active
// listings whenever the file changes and lets the processor dedupe by content
// hash. A conditional GET (ETag) avoids re-emitting when the file is unchanged.
//
// It also implements collector.Backfiller (see backfill.go): walking git history
// to reconstruct multiple years of posting-timing data.
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
	"os"
	"time"

	"github.com/Mekski/reqradar/internal/collector"
	"github.com/Mekski/reqradar/internal/signal"
	"github.com/Mekski/reqradar/internal/store"
)

type config struct {
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	ListingsPath string `json:"listings_path"`
}

type Collector struct {
	client       *http.Client
	owner        string
	repo         string
	listingsPath string
	token        string // GITHUB_TOKEN, used only for backfill (history API); polling is public
	etag         string // in-memory conditional-GET state; a restart re-fetches once
	log          *slog.Logger
}

// New is the collector.Factory for simplify-listings. The store arg is unused
// (this aggregator's config is fully static) — it's part of the shared Factory
// signature.
func New(raw json.RawMessage, _ *store.Store, log *slog.Logger) (collector.Collector, error) {
	var cfg config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Owner == "" || cfg.Repo == "" || cfg.ListingsPath == "" {
		return nil, fmt.Errorf("owner, repo, and listings_path are required")
	}
	return &Collector{
		client:       &http.Client{Timeout: 60 * time.Second},
		owner:        cfg.Owner,
		repo:         cfg.Repo,
		listingsPath: cfg.ListingsPath,
		token:        os.Getenv("GITHUB_TOKEN"),
		log:          log,
	}, nil
}

func (c *Collector) Name() string            { return "simplify-listings" }
func (c *Collector) Schedule() time.Duration { return 5 * time.Minute }

// rawURL builds the raw.githubusercontent URL for the listings file at a ref
// (branch name for polling, commit SHA for backfill snapshots).
func (c *Collector) rawURL(ref string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", c.owner, c.repo, ref, c.listingsPath)
}

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.rawURL("dev"), nil)
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
	listings, err := parseListings(body)
	if err != nil {
		return nil, err
	}
	c.etag = resp.Header.Get("ETag")

	now := time.Now()
	var signals []signal.RawSignal
	for i := range listings {
		l := listings[i]
		if !l.Active {
			continue // polling emits only currently-open postings; history is backfill's job
		}
		signals = append(signals, toSignal(c.Name(), l, now))
	}
	return signals, nil
}

func parseListings(body []byte) ([]listing, error) {
	var listings []listing
	if err := json.Unmarshal(body, &listings); err != nil {
		return nil, fmt.Errorf("parse listings: %w", err)
	}
	return listings, nil
}

// toSignal builds a RawSignal from a listing. EventTime is the posting's own
// date_posted (historical during backfill); observedAt is when we ingested it.
func toSignal(source string, l listing, observedAt time.Time) signal.RawSignal {
	payload, _ := json.Marshal(l)
	return signal.RawSignal{
		Source:      source,
		ExternalID:  l.ID,
		Kind:        signal.KindPosting,
		EventTime:   time.Unix(l.DatePosted, 0).UTC(),
		ObservedAt:  observedAt,
		Payload:     payload,
		ContentHash: contentHash(l),
	}
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
