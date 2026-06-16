// Package ats discovers which Applicant Tracking System board (Greenhouse or
// Ashby) a watchlist company uses, via grounded web search, so its postings get
// the rich pipeline (pay + JD text + live detection) without a human looking up
// the board slug. A discovered slug is VERIFIED against the live ATS API before
// it's stored — a hallucinated slug 404s and is discarded, so we never poll a
// dead or wrong-company board. Reached on-demand (company added, or the
// discover-ats backfill); never on the alert path.
package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Mekski/reqradar/internal/llm"
	"github.com/Mekski/reqradar/internal/store"
)

type Service struct {
	llm    llm.Client
	store  *store.Store
	client *http.Client
}

func New(c llm.Client, st *store.Store) *Service {
	return &Service{llm: c, store: st, client: &http.Client{Timeout: 15 * time.Second}}
}

// Configured reports whether discovery can run (an API key is set).
func (s *Service) Configured() bool { return s.llm.Configured() }

// Discover finds and stores a company's ATS board, but only when it has none yet.
// Fully non-fatal and safe fire-and-forget: an unknown platform, an unparseable
// answer, or a slug that fails live verification all leave the company as-is.
func (s *Service) Discover(ctx context.Context, entityID int64, name string) error {
	if !s.Configured() {
		return nil
	}
	if _, slug, err := s.store.EntityATS(ctx, entityID); err != nil {
		return err
	} else if slug != "" {
		return nil // already known
	}

	text, _, err := s.llm.GenerateGrounded(ctx, buildPrompt(name))
	if err != nil {
		return fmt.Errorf("grounded discovery: %w", err)
	}
	platform, slug := parseDiscovery(text)
	if platform == "" || slug == "" {
		return nil // none / unparseable — don't guess
	}
	if !s.verify(ctx, platform, slug) {
		return nil // the board didn't resolve — discard a likely-hallucinated slug
	}
	return s.store.SetEntityATS(ctx, entityID, platform, slug)
}

// slugRE guards against junk/hallucinated tokens before we spend an HTTP verify on
// them: a board slug is a single lowercase URL path segment.
var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// parseDiscovery pulls "PLATFORM:" and "SLUG:" out of the model's reply. Returns
// ("","") for platform "none" or anything that isn't a clean greenhouse/ashby slug.
func parseDiscovery(text string) (platform, slug string) {
	for _, line := range strings.Split(text, "\n") {
		low := strings.ToLower(line)
		if i := strings.Index(low, "platform:"); i >= 0 {
			platform = strings.Trim(strings.TrimSpace(low[i+len("platform:"):]), ".*_` ")
		}
		if i := strings.Index(low, "slug:"); i >= 0 {
			slug = strings.Trim(strings.TrimSpace(low[i+len("slug:"):]), ".*_`/ ")
		}
	}
	if platform != "greenhouse" && platform != "ashby" {
		return "", ""
	}
	if !slugRE.MatchString(slug) {
		return "", ""
	}
	return platform, slug
}

// verify confirms the board actually exists on the named platform by hitting the
// same public API the collector polls. This is the anti-hallucination backstop:
// only a slug that returns a real board (HTTP 200 + a decodable jobs payload) is
// trusted.
func (s *Service) verify(ctx context.Context, platform, slug string) bool {
	var url string
	switch platform {
	case "greenhouse":
		url = fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs", slug)
	case "ashby":
		url = fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", slug)
	default:
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false
	}
	// A valid board response is a JSON object with a "jobs" array (possibly empty
	// if nothing is open right now — still a real board).
	var probe struct {
		Jobs *[]json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.Jobs == nil {
		return false
	}
	return true
}
