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
	path := "seed/watchlist.yaml"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	var sf seedFile
	if err := yaml.Unmarshal(raw, &sf); err != nil {
		log.Fatalf("parse %s: %v", path, err)
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

	atsOrgs := map[string][]string{} // platform -> slugs, derived from companies
	for _, c := range sf.Companies {
		aliases := append([]string{}, c.Aliases...)
		if c.ATS != nil {
			aliases = append(aliases, c.ATS.Slug)
		}
		if _, err := st.UpsertCompany(ctx, tx, userID, store.CompanyInput{
			Name: c.CanonicalName, Domain: c.Domain, Priority: c.Priority, Source: "seed",
			Aliases: aliases, ExpectedEstimate: c.ExpectedEstimate,
		}); err != nil {
			log.Fatalf("upsert company %s: %v", c.CanonicalName, err)
		}
		if c.ATS != nil {
			atsOrgs[c.ATS.Platform] = append(atsOrgs[c.ATS.Platform], c.ATS.Slug)
		}
	}

	for platform, slugs := range atsOrgs {
		upsertSource(ctx, tx, source{
			Name: platform, Kind: "ats", Enabled: true,
			Config: map[string]any{"orgs": slugs},
		})
	}
	for _, s := range sf.Sources {
		upsertSource(ctx, tx, s)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit: %v", err)
	}
	log.Printf("seeded: %d companies, %d ATS platforms, %d static sources",
		len(sf.Companies), len(atsOrgs), len(sf.Sources))
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
