package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Mekski/reqradar/internal/backfill"
)

// runBackfill kicks off a history rebuild (the dashboard's "rebuild history"
// button). It returns immediately — a backfill takes ~30s, too long to hold the
// request — and the UI polls /api/backfill/status. A second click while one is
// running gets a 409, not a duplicate run.
func (s *Server) runBackfill(w http.ResponseWriter, _ *http.Request) {
	if s.backfill == nil {
		http.Error(w, "backfill not available", http.StatusServiceUnavailable)
		return
	}
	// Detached from the request so the ~30s run isn't cancelled when the POST
	// returns; bounded so a wedged run can't leak forever.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := s.backfill.Run(ctx); err != nil && !errors.Is(err, backfill.ErrAlreadyRunning) {
			s.log.Error("backfill run", "err", err)
		}
	}()
	s.respond(w, map[string]bool{"started": true}, nil)
}

// backfillStatus reports whether a backfill is running and the last run's result,
// so the button can show "running…" then "done · just now".
func (s *Server) backfillStatus(w http.ResponseWriter, _ *http.Request) {
	if s.backfill == nil {
		s.respond(w, backfill.Status{}, nil)
		return
	}
	s.respond(w, s.backfill.Status(), nil)
}
