// Package sentiment generates an on-demand, web-search-grounded "what does the
// community say" report per company (prestige, culture, interview process, intern
// pay/housing, return offers). One report is stored per company; regenerating
// replaces it. The LLM (grounded Gemini) is reached only on a user button press —
// never automatically, never on the alert path.
package sentiment

import (
	"context"
	"encoding/json"

	"github.com/Mekski/reqradar/internal/llm"
	"github.com/Mekski/reqradar/internal/store"
)

type Service struct {
	llm   llm.Client
	store *store.Store
}

func New(c llm.Client, st *store.Store) *Service { return &Service{llm: c, store: st} }

// Configured reports whether generation is available (an API key is set).
func (s *Service) Configured() bool { return s.llm.Configured() }

// Get returns the stored report for a company, or nil if none was generated yet.
func (s *Service) Get(ctx context.Context, entityID int64) (*store.Sentiment, error) {
	return s.store.GetSentiment(ctx, entityID)
}

// Generate runs a fresh grounded report and stores it (replacing any prior one).
func (s *Service) Generate(ctx context.Context, entityID int64) (*store.Sentiment, error) {
	name, err := s.store.CompanyName(ctx, entityID)
	if err != nil {
		return nil, err
	}
	text, sources, err := s.llm.GenerateGrounded(ctx, buildPrompt(name))
	if err != nil {
		return nil, err
	}
	if sources == nil {
		sources = []llm.Source{}
	}
	srcJSON, _ := json.Marshal(sources)
	return s.store.UpsertSentiment(ctx, entityID, text, srcJSON, s.llm.Model())
}
