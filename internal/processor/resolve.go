package processor

import (
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/Mekski/reqradar/internal/entity"
)

// Resolver maps a raw company string to a watchlist entity using the
// deterministic part of the cascade (exact/alias, then domain). The LLM step for
// the ambiguous long tail is a later task; until then a miss resolves to "not a
// watchlist entity", which is correct for the ~93% of aggregator postings that
// are not watchlist companies anyway. See DESIGN.md §6.
//
// The lookup tables live behind an atomic pointer so Reload can swap in a fresh
// snapshot (e.g. after a company is added via the dashboard) without a restart
// and without locking the hot Resolve path.
type Resolver struct {
	tables atomic.Pointer[tables]
}

type tables struct {
	aliases map[string]int64 // normalized alias -> entity_id
	domains map[string]int64 // lowercased domain -> entity_id
}

func NewResolver(aliases, domains map[string]int64) *Resolver {
	r := &Resolver{}
	r.tables.Store(&tables{aliases: aliases, domains: domains})
	return r
}

// Reload atomically replaces the lookup tables.
func (r *Resolver) Reload(aliases, domains map[string]int64) {
	r.tables.Store(&tables{aliases: aliases, domains: domains})
}

// Resolve returns the entity id, the method used, and whether it resolved to a
// watchlist entity.
func (r *Resolver) Resolve(company, rawURL string) (entityID int64, method string, ok bool) {
	t := r.tables.Load()
	if id, found := t.aliases[entity.Normalize(company)]; found {
		return id, "alias", true
	}
	if host := hostOf(rawURL); host != "" {
		if id, found := t.domains[host]; found {
			return id, "domain", true
		}
	}
	return 0, "none", false
}

// hostOf extracts a registrable-ish host from a URL: lowercased, www. stripped.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}
