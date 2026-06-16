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

// Client generates a JSON completion from a prompt. Implementations request
// JSON-only output from the provider; the caller validates/parses the bytes.
type Client interface {
	// GenerateJSON returns the model's raw JSON response for prompt.
	GenerateJSON(ctx context.Context, prompt string) ([]byte, error)
	// Model is the provider model id, recorded with cached results.
	Model() string
	// Configured reports whether an API key is present (false => GenerateJSON
	// returns ErrNotConfigured). Lets handlers answer before doing work.
	Configured() bool
}
