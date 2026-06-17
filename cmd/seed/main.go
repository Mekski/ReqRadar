// Command seed loads the watchlist config (seed/watchlist.yaml) into the entity
// registry, watchlist, and source config. Idempotent: re-run after editing the
// YAML to add companies. This is why the watchlist lives in config, not a
// migration — it is data that changes, and migrations are forward-only.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"github.com/Mekski/reqradar/internal/config"
	"github.com/Mekski/reqradar/internal/store"
	"github.com/Mekski/reqradar/seed"
)

type seedFile struct {
	User struct {
		Name string `yaml:"name"`
	} `yaml:"user"`
	Companies []company `yaml:"companies"`
	Sources   []source  `yaml:"sources"`
}

type company struct {
	CanonicalName    string   `yaml:"canonical_name"`
	Domain           string   `yaml:"domain"`
	Priority         string   `yaml:"priority"`
	ExpectedEstimate string   `yaml:"expected_estimate"` // curated summer-SWE open month (fallback when data is sparse)
	Aliases          []string `yaml:"aliases"`
	ATS              *struct {
		Platform string `yaml:"platform"`
		Slug     string `yaml:"slug"`
	} `yaml:"ats"`
}

type source struct {
	Name    string         `yaml:"name"`
	Kind    string         `yaml:"kind"`
	Enabled bool           `yaml:"enabled"`
	Config  map[string]any `yaml:"config"`
}

func main() {
	// Default to the embedded watchlist (self-contained binary, works in a container
	// with no source checkout). An explicit path arg overrides it for local edits.
	raw := seed.Watchlist
	src := "embedded watchlist.yaml"
	if len(os.Args) > 1 {
		src = os.Args[1]
		var err error
		if raw, err = os.ReadFile(src); err != nil {
			log.Fatalf("read %s: %v", src, err)
		}
	}
	var sf seedFile
	if err := yaml.Unmarshal(raw, &sf); err != nil {
		log.Fatalf("parse %s: %v", src, err)
	}

	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if chatID == "" {
		log.Print("warning: TELEGRAM_CHAT_ID not set — seeding a placeholder; re-run seed once it is set")
		chatID = "PLACEHOLDER"
	}

	ctx := context.Background()
	st, err := store.Open(ctx, config.Load().PostgresDSN)
	if err != nil {
		log.Fatalf("store open: %v", err)
	}
	defer st.Close()

	tx, err := st.Pool.Begin(ctx)
	if err != nil {
		log.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	userID := upsertUser(ctx, tx, sf.User.Name, chatID)

	atsPlatforms := map[string]bool{} // platforms in use, to enable their collectors
	for _, c := range sf.Companies {
		aliases := append([]string{}, c.Aliases...)
		in := store.CompanyInput{
			Name: c.CanonicalName, Domain: c.Domain, Priority: c.Priority, Source: "seed",
			Aliases: aliases, ExpectedEstimate: c.ExpectedEstimate,
		}
		if c.ATS != nil {
			aliases = append(aliases, c.ATS.Slug)
			in.Aliases = aliases
			// Store the board on the entity — the single source of truth the ATS
			// collectors read each poll cycle (entities.metadata.ats). Runtime
			// discovery writes the same field, so seed + discovery never diverge.
			in.ATSPlatform, in.ATSSlug = c.ATS.Platform, c.ATS.Slug
			atsPlatforms[c.ATS.Platform] = true
		}
		if _, err := st.UpsertCompany(ctx, tx, userID, in); err != nil {
			log.Fatalf("upsert company %s: %v", c.CanonicalName, err)
		}
	}

	// Enable each ATS platform's collector. The poll-list is NOT in this config —
	// the collectors read it from entities.metadata.ats — so config stays empty.
	for platform := range atsPlatforms {
		upsertSource(ctx, tx, source{
			Name: platform, Kind: "ats", Enabled: true, Config: map[string]any{},
		})
	}
	for _, s := range sf.Sources {
		upsertSource(ctx, tx, s)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit: %v", err)
	}
	log.Printf("seeded: %d companies, %d ATS platforms, %d static sources",
		len(sf.Companies), len(atsPlatforms), len(sf.Sources))
}

func upsertUser(ctx context.Context, tx pgx.Tx, name, chatID string) int64 {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE name = $1`, name).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if err := tx.QueryRow(ctx,
			`INSERT INTO users (name, telegram_chat_id) VALUES ($1, $2) RETURNING id`,
			name, chatID).Scan(&id); err != nil {
			log.Fatalf("insert user: %v", err)
		}
	case err != nil:
		log.Fatalf("select user: %v", err)
	default:
		if _, err := tx.Exec(ctx, `UPDATE users SET telegram_chat_id = $2 WHERE id = $1`, id, chatID); err != nil {
			log.Fatalf("update user: %v", err)
		}
	}
	return id
}

func upsertSource(ctx context.Context, tx pgx.Tx, s source) {
	cfg, _ := json.Marshal(s.Config)
	if _, err := tx.Exec(ctx,
		`INSERT INTO sources (name, kind, config, enabled) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (name) DO UPDATE SET kind = EXCLUDED.kind, config = EXCLUDED.config, enabled = EXCLUDED.enabled`,
		s.Name, s.Kind, cfg, s.Enabled); err != nil {
		log.Fatalf("upsert source %s: %v", s.Name, err)
	}
}
