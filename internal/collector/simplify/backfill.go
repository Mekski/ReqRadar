package simplify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Mekski/reqradar/internal/signal"
)

// sampleInterval is how far apart we sample git-history snapshots. The current
// listings.json only covers the live cycle (~8 months), so multi-year history
// requires reading the file at past commits. We sample rather than walk all
// ~24k commits: each posting carries its own date_posted, so a snapshot every
// ~60 days reconstructs the timing distribution. Tradeoff (documented, not
// silent): a posting that opened AND closed entirely between two samples is
// missed — acceptable, since internship postings persist for weeks and we only
// need the date_posted distribution, not every individual row.
const sampleInterval = 60 * 24 * time.Hour

// Backfill reconstructs historical postings by sampling git-history snapshots of
// listings.json between from and to, deduping by posting id, and emitting each
// once. Implements collector.Backfiller. Requires GITHUB_TOKEN for comfortable
// API rate limits (falls back to unauthenticated, which is tight).
func (c *Collector) Backfill(ctx context.Context, from, to time.Time, emit func(signal.RawSignal) error) error {
	seen := map[string]bool{}    // posting id -> emitted
	seenSHA := map[string]bool{} // commit sha -> processed
	now := time.Now()
	snapshots, emitted := 0, 0

	for d := to; d.After(from); d = d.Add(-sampleInterval) {
		sha, err := c.commitAt(ctx, d)
		if err != nil {
			c.log.Warn("backfill: commit lookup failed", "date", d.Format("2006-01-02"), "err", err)
			continue
		}
		if sha == "" || seenSHA[sha] {
			continue
		}
		seenSHA[sha] = true

		listings, err := c.snapshotAt(ctx, sha)
		if err != nil {
			c.log.Warn("backfill: snapshot fetch failed", "sha", short(sha), "err", err)
			continue
		}
		snapshots++

		fresh := 0
		for i := range listings {
			l := listings[i]
			if l.ID == "" || seen[l.ID] {
				continue
			}
			seen[l.ID] = true
			sig := toSignal(c.Name(), l, now)
			sig.Backfill = true // historical replay — processor skips the firehose for these
			if err := emit(sig); err != nil {
				return fmt.Errorf("emit: %w", err)
			}
			emitted++
			fresh++
		}
		c.log.Info("backfill: snapshot processed", "date", d.Format("2006-01"),
			"sha", short(sha), "new_postings", fresh, "unique_total", len(seen))
	}

	c.log.Info("backfill complete", "snapshots", snapshots, "unique_postings", len(seen), "emitted", emitted)
	return nil
}

// commitAt returns the SHA of the most recent commit touching the listings file
// at or before t, or "" if none.
func (c *Collector) commitAt(ctx context.Context, t time.Time) (string, error) {
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?path=%s&until=%s&per_page=1",
		c.owner, c.repo, url.QueryEscape(c.listingsPath), t.UTC().Format(time.RFC3339))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("commits api status %d", resp.StatusCode)
	}

	var commits []struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return "", err
	}
	if len(commits) == 0 {
		return "", nil
	}
	return commits[0].SHA, nil
}

// snapshotAt fetches and parses the listings file at a specific commit.
func (c *Collector) snapshotAt(ctx context.Context, sha string) ([]listing, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.rawURL(sha), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("raw status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseListings(body)
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
