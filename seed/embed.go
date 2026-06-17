// Package seed embeds the watchlist config so the seed binary is self-contained
// (no loose watchlist.yaml to ship in the container image) — mirroring how the
// migrate binary embeds its .sql files. A CI-built image can therefore seed with
// no source checkout on the host.
package seed

import _ "embed"

//go:embed watchlist.yaml
var Watchlist []byte
