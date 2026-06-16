package api

import (
	"errors"
	"net/http"

	"github.com/Mekski/reqradar/internal/llm"
)

// getSentiment returns the stored report for a company (or {configured} + null so
// the UI knows whether to show a "generate" button vs a "not configured" hint).
func (s *Server) getSentiment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	report, err := s.sentiment.Get(r.Context(), id)
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	s.respond(w, map[string]any{"configured": s.sentiment.Configured(), "report": report}, nil)
}

// generateSentiment runs a fresh grounded report (replacing any prior one). This
// is the only path that hits the LLM, and only on an explicit button press.
func (s *Server) generateSentiment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// Detach so a closed tab still completes + stores the grounded report (a
	// grounded search isn't free; don't throw it away on disconnect).
	genCtx, cancel := detachedLLMCtx(r.Context())
	defer cancel()
	report, err := s.sentiment.Generate(genCtx, id)
	if errors.Is(err, llm.ErrNotConfigured) {
		http.Error(w, "sentiment isn't configured yet — add GEMINI_API_KEY to .env and restart the api", http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		s.log.Error("generate sentiment", "entity", id, "err", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.respond(w, report, nil)
}
