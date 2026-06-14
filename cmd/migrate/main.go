// Command migrate applies or rolls back database migrations. The SQL is embedded
// (see package migrations), so this binary is self-contained for the compose init
// step. Usage: migrate [up|down|version]. See DESIGN.md §7.
package main

import (
	"errors"
	"log"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/Mekski/reqradar/internal/config"
	"github.com/Mekski/reqradar/migrations"
)

func main() {
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		log.Fatalf("load migrations: %v", err)
	}

	// golang-migrate's pgx/v5 driver registers the pgx5:// URL scheme.
	dbURL := strings.Replace(config.Load().PostgresDSN, "postgres://", "pgx5://", 1)
	m, err := migrate.NewWithSourceInstance("iofs", src, dbURL)
	if err != nil {
		log.Fatalf("init migrate: %v", err)
	}
	defer m.Close()

	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	case "version":
		v, dirty, verr := m.Version()
		if verr != nil {
			log.Fatalf("version: %v", verr)
		}
		log.Printf("version=%d dirty=%v", v, dirty)
		return
	default:
		log.Fatalf("unknown command %q (use: up | down | version)", cmd)
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("%s: %v", cmd, err)
	}
	log.Printf("%s: ok", cmd)
}
