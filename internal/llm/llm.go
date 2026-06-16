// Package llm wraps free-tier LLM providers for the non-hot-path enrichment
// features (fit score today). The Client interface keeps callers provider-
// agnostic; Gemini is the first implementation. Per DESIGN, the LLM is never on
// the alert path — only on-demand, cached-forever requests reach it.
package llm

import (
	"context"
	"errors"
)

// ErrNotConfigured is returned when no API key is set, so callers can surface a
// clear "add GEMINI_API_KEY" message instead of a generic 500.
var ErrNotConfigured = errors.New("LLM not configured (set GEMINI_API_KEY)")

// Source is a citation surfaced by grounded generation. These come from the
// provider's grounding metadata (the real URLs it searched) — never model-authored
// text — so they can't be hallucinated.
type Source struct {
	Title string `json:"title"`
	URI   string `json:"uri"`
}

// Client generates completions from a prompt. Implementations request JSON-only
// output (GenerateJSON) or run a web-search-grounded generation (GenerateGrounded).
type Client interface {
	// GenerateJSON returns the model's raw JSON response for prompt.
	GenerateJSON(ctx context.Context, prompt string) ([]byte, error)
	// GenerateGrounded runs the prompt with web-search grounding, returning the
	// generated text plus the real sources the model grounded on.
	GenerateGrounded(ctx context.Context, prompt string) (string, []Source, error)
	// Model is the provider model id, recorded with cached results.
	Model() string
	// Configured reports whether an API key is present (false => calls return
	// ErrNotConfigured). Lets handlers answer before doing work.
	Configured() bool
}
