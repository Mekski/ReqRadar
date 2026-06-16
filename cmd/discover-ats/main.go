// Command discover-ats fills the ATS board (Greenhouse/Ashby slug) for watchlist
// companies that don't have one yet, using the same grounded-search + live-verify
// discovery that runs automatically when a company is added. Run it once after
// adding companies in bulk; the ATS collectors pick up the new slugs on their next
// poll (no restart). Needs GEMINI_API_KEY; without it, it no-ops.
package main

import (
	"context"
	"time"

	"github.com/Mekski/reqradar/internal/ats"
	"github.com/Mekski/reqradar/internal/llm"
	"github.com/Mekski/reqradar/internal/service"
	"github.com/Mekski/reqradar/internal/store"
)

func main() {
	ctx, cfg, log, stop := service.Bootstrap("discover-ats")
	defer stop()

	st, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Error("store open", "err", err)
		return
	}
	defer st.Close()

	svc := ats.New(llm.NewGemini(cfg.GeminiKey, cfg.GeminiModel), st)
	if !svc.Configured() {
		log.Error("GEMINI_API_KEY not set — nothing to do")
		return
	}

	userID, err := st.FirstUserID(ctx)
	if err != nil {
		log.Error("no user found — run the seed first", "err", err)
		return
	}
	targets, err := st.WatchlistEntitiesWithoutATS(ctx, userID)
	if err != nil {
		log.Error("load companies", "err", err)
		return
	}
	log.Info("discovering ATS boards for companies with none", "count", len(targets))

	for _, c := range targets {
		select {
		case <-ctx.Done():
			return
		default:
		}
		callCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
		err := svc.Discover(callCtx, c.ID, c.Name)
		cancel()
		if err != nil {
			log.Warn("discovery failed", "company", c.Name, "err", err)
			continue
		}
		platform, slug, _ := st.EntityATS(ctx, c.ID)
		if slug == "" {
			log.Info("no verified ATS board found — left blank", "company", c.Name)
		} else {
			log.Info("ATS board set", "company", c.Name, "platform", platform, "slug", slug)
		}
	}
	log.Info("done")
}
