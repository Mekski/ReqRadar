package processor

import "encoding/json"

// Posting is the normalized, source-agnostic view of a job posting that the rest
// of the pipeline works with. Per-source Normalizers produce it from raw payloads.
type Posting struct {
	Company   string   `json:"company"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Locations []string `json:"locations"`
	Terms     []string `json:"terms"`
	Category  string   `json:"category"`
}

// Normalizer parses a source-native payload into a Posting. Normalization is the
// processor's job (collectors stay dump/pass-through), so all source-specific
// parsing lives here.
type Normalizer func(payload []byte) (Posting, error)

// normalizeSimplify parses a SimplifyJobs listings.json entry. The struct mirrors
// the wire format the simplify collector emits (intentionally a local copy: the
// wire format, not the Go type, is the contract; golden-file tests guard drift).
func normalizeSimplify(payload []byte) (Posting, error) {
	var l struct {
		CompanyName string   `json:"company_name"`
		Title       string   `json:"title"`
		URL         string   `json:"url"`
		Locations   []string `json:"locations"`
		Terms       []string `json:"terms"`
		Category    string   `json:"category"`
	}
	if err := json.Unmarshal(payload, &l); err != nil {
		return Posting{}, err
	}
	return Posting{
		Company:   l.CompanyName,
		Title:     l.Title,
		URL:       l.URL,
		Locations: l.Locations,
		Terms:     l.Terms,
		Category:  l.Category,
	}, nil
}
