// Package expected researches a company's typical Summer-SWE-intern application
// opening month via grounded web search, for companies too sparse to derive it
// from posting history. It's the automated version of the manual "curated
// estimate" pass: triggered on-demand (a company is added, or a backfill command),
// stored once with its source citation, never on the alert path. The grounded
// model is reached at most once per company that lacks an estimate.
package expected

import (
	"context"
	"fmt"
	"strings"

	"github.com/Mekski/reqradar/internal/llm"
	"github.com/Mekski/reqradar/internal/store"
)

type Service struct {
	llm   llm.Client
	store *store.Store
}

func New(c llm.Client, st *store.Store) *Service { return &Service{llm: c, store: st} }

// Configured reports whether research can run (an API key is set).
func (s *Service) Configured() bool { return s.llm.Configured() }

// Research fills a company's expected-open estimate from a grounded search, but
// only when it has none yet (so it never overwrites a hand-curated estimate or
// repeats the call). A blank/unknown result is left blank — the UI then shows "—"
// just as before. Safe to call fire-and-forget: all outcomes are non-fatal.
func (s *Service) Research(ctx context.Context, entityID int64, name string) error {
	if !s.Configured() {
		return nil
	}
	existing, err := s.store.CompanyExpectedEstimate(ctx, entityID)
	if err != nil {
		return err
	}
	if existing != "" {
		return nil // already has a curated or previously-researched estimate
	}

	text, sources, err := s.llm.GenerateGrounded(ctx, buildPrompt(name))
	if err != nil {
		return fmt.Errorf("grounded research: %w", err)
	}
	month, ok := parseAnswer(text)
	if !ok {
		return nil // "unknown" / unparseable — leave blank, don't guess
	}
	url := ""
	if len(sources) > 0 {
		url = sources[0].URI
	}
	return s.store.SetExpectedEstimate(ctx, entityID, month, "llm", url)
}

// validMonths is the set of accepted answers (besides "rolling"); the stored value
// matches the "Sep"/"Oct" format the curated seed + the UI card already use.
var validMonths = map[string]string{
	"jan": "Jan", "feb": "Feb", "mar": "Mar", "apr": "Apr", "may": "May", "jun": "Jun",
	"jul": "Jul", "aug": "Aug", "sep": "Sep", "oct": "Oct", "nov": "Nov", "dec": "Dec",
}

// parseAnswer pulls the month / "rolling" out of the model's "ANSWER: <x>" line.
// Returns ("", false) for "unknown" or anything it can't validate, so a low-
// confidence answer leaves the estimate blank rather than inventing a month.
func parseAnswer(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		i := strings.Index(lower, "answer:")
		if i < 0 {
			continue // tolerates leading markdown like "**ANSWER:**"
		}
		// Token after "answer:", stripped of markdown/punctuation/whitespace.
		tok := strings.Trim(strings.TrimSpace(lower[i+len("answer:"):]), ".*_ \t")
		if tok == "" {
			continue
		}
		if strings.HasPrefix(tok, "rolling") {
			return "rolling", true
		}
		if m, ok := validMonths[tok[:min(3, len(tok))]]; ok {
			return m, true
		}
		return "", false // "unknown" or unexpected
	}
	return "", false
}
