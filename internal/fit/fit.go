// Package fit scores a resume against a job description with a free-tier LLM,
// caching every result forever (one model call per unique (jd, resume) pair). The
// LLM is reached only here, on-demand — never on the alert path.
package fit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// Configured reports whether scoring is available (an API key is set).
func (s *Service) Configured() bool { return s.llm.Configured() }

// Score returns the fit result JSON for (resumeText, jdText), from the cache when
// the pair was scored before, else from a fresh model call (then cached).
// postingID is recorded for provenance when the JD came from a watchlist posting.
// The second return is true when served from cache.
func (s *Service) Score(ctx context.Context, resumeText, jdText string, postingID *int64) (json.RawMessage, bool, error) {
	resumeText, jdText = strings.TrimSpace(resumeText), strings.TrimSpace(jdText)
	if resumeText == "" || jdText == "" {
		return nil, false, fmt.Errorf("resume and job description are both required")
	}
	// Fold the prompt version AND the model id into the JD hash so a prompt/rubric
	// change OR a model switch (e.g. flash->pro) invalidates stale cached scores
	// instead of serving a grade the current model+prompt never produced.
	jdHash, resumeHash := hash(promptVersion+"\x00"+s.llm.Model()+"\x00"+jdText), hash(resumeText)

	if cached, ok, err := s.store.GetFitScore(ctx, jdHash, resumeHash); err != nil {
		return nil, false, err
	} else if ok {
		return cached, true, nil
	}

	prompt := buildPrompt(jdText, resumeText)
	// Generate, validating the shape before caching. Retry once if the model
	// returns malformed JSON (a rare hiccup even with the schema + no-thinking config).
	var raw json.RawMessage
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		raw, lastErr = s.llm.GenerateJSON(ctx, prompt, nil)
		if lastErr != nil {
			return nil, false, lastErr // API/config error — don't retry
		}
		var r Result
		if lastErr = json.Unmarshal(raw, &r); lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return nil, false, fmt.Errorf("model returned malformed JSON: %w", lastErr)
	}
	if err := s.store.SaveFitScore(ctx, jdHash, resumeHash, postingID, s.llm.Model(), raw); err != nil {
		return nil, false, err
	}
	return raw, false, nil
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
