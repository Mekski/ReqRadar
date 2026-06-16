// Command enrich-expected fills the expected-open estimate for watchlist
// companies that don't have one yet (no curated estimate, too little posting
// history to derive it), using the same grounded-search research that runs
// automatically when a company is added. Run it once after adding companies in
// bulk (e.g. the currently-blank Airbnb/Coinbase/Discord/Figma/Snap/Spotify/Stripe).
// Needs GEMINI_API_KEY; without it, it no-ops.
package main

import (
	"context"
	"time"

	"github.com/Mekski/reqradar/internal/expected"
	"github.com/Mekski/reqradar/internal/llm"
	"github.com/Mekski/reqradar/internal/service"
	"github.com/Mekski/reqradar/internal/store"
)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("enrich-expected")
	defer stop()

	st, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Error("store open", "err", err)
		return
	}
	defer st.Close()

	svc := expected.New(llm.NewGemini(cfg.GeminiKey, cfg.GeminiModel), st)
	if !svc.Configured() {
		log.Error("GEMINI_API_KEY not set — nothing to do")
		return
	}

	userID, err := st.FirstUserID(ctx)
	if err != nil {
		log.Error("no user found — run the seed first", "err", err)
		return
	}
	targets, err := st.BlankEstimateCompanies(ctx, userID)
	if err != nil {
		log.Error("load companies", "err", err)
		return
	}
	log.Info("researching expected-open for companies with no estimate", "count", len(targets))

	for _, c := range targets {
		select {
		case <-ctx.Done():
			return
		default:
		}
		callCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
		err := svc.Research(callCtx, c.ID, c.Name)
		cancel()
		if err != nil {
			log.Warn("research failed", "company", c.Name, "err", err)
			continue
		}
		est, _ := st.CompanyExpectedEstimate(ctx, c.ID)
		if est == "" {
			log.Info("no reliable estimate found — left blank", "company", c.Name)
		} else {
			log.Info("estimate set", "company", c.Name, "month", est)
		}
	}
	log.Info("done")
}
