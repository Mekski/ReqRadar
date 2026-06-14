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
	"github.com/Mekski/reqradar/internal/entity"
)

type seedFile struct {
	User struct {
		Name string `yaml:"name"`
	} `yaml:"user"`
	Companies []company `yaml:"companies"`
	Sources   []source  `yaml:"sources"`
}

type company struct {
	CanonicalName string   `yaml:"canonical_name"`
	Domain        string   `yaml:"domain"`
	Priority      string   `yaml:"priority"`
	Aliases       []string `yaml:"aliases"`
	ATS           *struct {
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
	conn, err := pgx.Connect(ctx, config.Load().PostgresDSN)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		log.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	userID := upsertUser(ctx, tx, sf.User.Name, chatID)

	atsOrgs := map[string][]string{} // platform -> slugs, derived from companies
	for _, c := range sf.Companies {
		entityID := upsertEntity(ctx, tx, c)
		upsertAliases(ctx, tx, entityID, aliasesFor(c))
		exec(ctx, tx,
			`INSERT INTO watchlist (user_id, entity_id, alert_config) VALUES ($1, $2, '{}')
			 ON CONFLICT (user_id, entity_id) DO NOTHING`,
			userID, entityID)
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
		mustScan(tx.QueryRow(ctx,
			`INSERT INTO users (name, telegram_chat_id) VALUES ($1, $2) RETURNING id`,
			name, chatID), &id)
	case err != nil:
		log.Fatalf("select user: %v", err)
	default:
		exec(ctx, tx, `UPDATE users SET telegram_chat_id = $2 WHERE id = $1`, id, chatID)
	}
	return id
}

func upsertEntity(ctx context.Context, tx pgx.Tx, c company) int64 {
	meta, _ := json.Marshal(map[string]any{"priority": c.Priority})
	var id int64
	mustScan(tx.QueryRow(ctx,
		`INSERT INTO entities (kind, canonical_name, domain, metadata)
		 VALUES ('company', $1, $2, $3)
		 ON CONFLICT (kind, canonical_name)
		 DO UPDATE SET domain = EXCLUDED.domain, metadata = EXCLUDED.metadata
		 RETURNING id`,
		c.CanonicalName, c.Domain, meta), &id)
	return id
}

// aliasesFor returns the normalized, de-duplicated alias set for a company:
// the canonical name, every listed alias, and the ATS slug if present.
func aliasesFor(c company) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		n := entity.Normalize(s)
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	add(c.CanonicalName)
	for _, a := range c.Aliases {
		add(a)
	}
	if c.ATS != nil {
		add(c.ATS.Slug)
	}
	return out
}

func upsertAliases(ctx context.Context, tx pgx.Tx, entityID int64, aliases []string) {
	for _, a := range aliases {
		exec(ctx, tx,
			`INSERT INTO entity_aliases (entity_id, alias, source, confidence)
			 VALUES ($1, $2, 'seed', 1.0)
			 ON CONFLICT (alias) DO UPDATE SET entity_id = EXCLUDED.entity_id, source = 'seed', confidence = 1.0`,
			entityID, a)
	}
}

func upsertSource(ctx context.Context, tx pgx.Tx, s source) {
	cfg, _ := json.Marshal(s.Config)
	exec(ctx, tx,
		`INSERT INTO sources (name, kind, config, enabled) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (name) DO UPDATE SET kind = EXCLUDED.kind, config = EXCLUDED.config, enabled = EXCLUDED.enabled`,
		s.Name, s.Kind, cfg, s.Enabled)
}

func exec(ctx context.Context, tx pgx.Tx, sql string, args ...any) {
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		log.Fatalf("exec: %v\n  sql: %s", err, sql)
	}
}

func mustScan(row pgx.Row, dest ...any) {
	if err := row.Scan(dest...); err != nil {
		log.Fatalf("scan: %v", err)
	}
}
