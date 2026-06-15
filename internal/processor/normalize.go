package processor

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Posting is the normalized, source-agnostic view of a job posting that the rest
// of the pipeline works with. Per-source Normalizers produce it from raw payloads.
type Posting struct {
	Company   string   `json:"company"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Locations []string `json:"locations"`
	Terms     []string `json:"terms"`
	Category  string   `json:"category"`

	// Posted pay, extracted from JD text when present. PayPeriod == "" means no
	// pay was found (or the source carries no JD text, e.g. SimplifyJobs).
	PayMin      float64 `json:"pay_min"`
	PayMax      float64 `json:"pay_max"`
	PayPeriod   string  `json:"pay_period"`
	PayCurrency string  `json:"pay_currency"`
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

// normalizeGreenhouse parses a greenhouse-collector payload. Greenhouse gives a
// title + departments but none of SimplifyJobs' structured terms/category, so we
// derive those here (interpretation belongs in the processor, not the collector).
// Company falls back to the org slug — seeded as an alias — when company_name is
// blank.
func normalizeGreenhouse(payload []byte) (Posting, error) {
	var e struct {
		Org         string   `json:"org"`
		Title       string   `json:"title"`
		CompanyName string   `json:"company_name"`
		URL         string   `json:"url"`
		Locations   []string `json:"locations"`
		Departments []string `json:"departments"`
		Content     string   `json:"content"`
	}
	if err := json.Unmarshal(payload, &e); err != nil {
		return Posting{}, err
	}
	company := e.CompanyName
	if company == "" {
		company = e.Org
	}
	p := Posting{
		Company:   company,
		Title:     e.Title,
		URL:       e.URL,
		Locations: e.Locations,
		Terms:     termsFromTitle(e.Title),
		Category:  greenhouseCategory(e.Title, e.Departments),
	}
	if pay, ok := extractPay(e.Content); ok {
		p.PayMin, p.PayMax, p.PayPeriod, p.PayCurrency = pay.Min, pay.Max, pay.Period, pay.Currency
	}
	return p, nil
}

var seasonRE = regexp.MustCompile(`(?i)\b(Summer|Fall|Winter|Spring)\b(?:\s+(\d{4}))?`)

// termsFromTitle extracts a "Summer 2027"-style term from a Greenhouse title so
// the processor's is_summer detection (Terms prefix "Summer") works the same way
// it does for SimplifyJobs. Returns nil when the title names no season — the role
// then simply doesn't count toward the summer-cohort seasonality, which is honest.
func termsFromTitle(title string) []string {
	m := seasonRE.FindStringSubmatch(title)
	if m == nil {
		return nil
	}
	season := strings.ToUpper(m[1][:1]) + strings.ToLower(m[1][1:]) // "SUMMER" -> "Summer"
	if m[2] != "" {
		return []string{season + " " + m[2]}
	}
	return []string{season}
}

// greenhouseCategory maps a Greenhouse title/departments onto the same category
// vocabulary SimplifyJobs uses, so the SWE seasonality filter
// (category IN ('Software','Software Engineering')) treats both sources alike.
// Best-effort and deliberately coarse; the messy-category cleanup is a later
// (LLM) task (CLAUDE.md LLM roadmap).
func greenhouseCategory(title string, departments []string) string {
	hay := strings.ToLower(title + " " + strings.Join(departments, " "))
	switch {
	case strings.Contains(hay, "software") || strings.Contains(hay, "swe"):
		return "Software Engineering"
	case strings.Contains(hay, "machine learning") || strings.Contains(hay, "ml ") ||
		strings.Contains(hay, "ai ") || strings.Contains(hay, "data scien") ||
		strings.Contains(hay, "applied scien") || strings.Contains(hay, "research scien"):
		return "AI/ML/Data"
	default:
		return ""
	}
}
