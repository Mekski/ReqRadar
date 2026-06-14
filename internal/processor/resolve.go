package processor

import (
	"net/url"
	"strings"

	"github.com/Mekski/reqradar/internal/entity"
)

// Resolver maps a raw company string to a watchlist entity using the
// deterministic part of the cascade (exact/alias, then domain). The LLM step for
// the ambiguous long tail is a later task; until then a miss resolves to "not a
// watchlist entity", which is correct for the ~93% of aggregator postings that
// are not watchlist companies anyway. See DESIGN.md §6.
type Resolver struct {
	aliases map[string]int64 // normalized alias -> entity_id
	domains map[string]int64 // lowercased domain -> entity_id
}

func NewResolver(aliases, domains map[string]int64) *Resolver {
	return &Resolver{aliases: aliases, domains: domains}
}

// Resolve returns the entity id, the method used, and whether it resolved to a
// watchlist entity.
func (r *Resolver) Resolve(company, rawURL string) (entityID int64, method string, ok bool) {
	if id, found := r.aliases[entity.Normalize(company)]; found {
		return id, "alias", true
	}
	if host := hostOf(rawURL); host != "" {
		if id, found := r.domains[host]; found {
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
