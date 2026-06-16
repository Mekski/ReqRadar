// Command firehose-prime records all currently-active firehose-category postings
// into firehose_seen WITHOUT alerting, so arming the firehose doesn't blast an
// alert for every one of the ~1000 postings already open. Run once before the
// first live run with the firehose enabled (job-watch's state-seed equivalent).
package main

import (
	"encoding/json"
	"time"

	"github.com/Mekski/reqradar/internal/collector/simplify"
	"github.com/Mekski/reqradar/internal/entity"
	"github.com/Mekski/reqradar/internal/service"
	"github.com/Mekski/reqradar/internal/store"
)

// firehoseCategories mirrors the processor's scope (SWE + AI/ML).
var firehoseCategories = map[string]bool{
	"Software":                            true,
	"Software Engineering":                true,
	"AI/ML/Data":                          true,
	"Data Science, AI & Machine Learning": true,
}

func main() {
	ctx, cfg, log, stop := service.Bootstrap("firehose-prime")
	defer stop()

	st, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Error("store open", "err", err)
		return
	}
	defer st.Close()

	sources, err := st.EnabledSources(ctx)
	if err != nil {
		log.Error("load sources", "err", err)
		return
	}
	var simplifyCfg json.RawMessage
	for _, s := range sources {
		if s.Name == "simplify-listings" {
			simplifyCfg = s.Config
		}
	}
	if simplifyCfg == nil {
		log.Error("simplify-listings source not found")
		return
	}

	c, err := simplify.New(simplifyCfg, st, log)
	if err != nil {
		log.Error("simplify init", "err", err)
		return
	}

	// Watchlist companies take the rich path, never the firehose — skip them here
	// so firehose_seen stays semantically "non-watchlist only".
	aliases, err := st.Aliases(ctx)
	if err != nil {
		log.Error("load aliases", "err", err)
		return
	}

	// Collect returns the currently-active postings as RawSignals.
	signals, err := c.Collect(ctx, time.Time{})
	if err != nil {
		log.Error("collect", "err", err)
		return
	}

	primed := 0
	for _, sig := range signals {
		var l struct {
			Company  string `json:"company_name"`
			Title    string `json:"title"`
			URL      string `json:"url"`
			Category string `json:"category"`
		}
		if err := json.Unmarshal(sig.Payload, &l); err != nil {
			continue
		}
		if !firehoseCategories[l.Category] {
			continue
		}
		if _, watchlisted := aliases[entity.Normalize(l.Company)]; watchlisted {
			continue // watchlist companies are handled by the rich path
		}
		if _, err := st.MarkFirehoseSeen(ctx, sig.Source, sig.ExternalID, l.Company, l.Title, l.URL, l.Category, sig.EventTime); err != nil {
			log.Error("mark seen", "id", sig.ExternalID, "err", err)
		}
		primed++
	}
	log.Info("firehose primed — existing backlog recorded silently", "postings", primed)
}
