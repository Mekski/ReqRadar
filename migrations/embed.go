// Package migrations embeds the SQL migration files so the migrate binary is
// self-contained — no loose .sql files to ship alongside it in the container.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
