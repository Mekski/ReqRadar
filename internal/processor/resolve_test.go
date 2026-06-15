package processor

import "testing"

// newTestResolver builds a resolver from literal tables. Alias keys must already
// be normalized (entity.Normalize), exactly as the seeder stores them, because
// Resolve normalizes only the incoming query before looking up.
func newTestResolver() *Resolver {
	return NewResolver(
		map[string]int64{
			"anthropic":  3,
			"riot games": 7,
			"google":     1,
		},
		map[string]int64{
			"anthropic.com": 3,
			"openai.com":    9,
		},
	)
}

func TestResolveCascade(t *testing.T) {
	r := newTestResolver()
	cases := []struct {
		name       string
		company    string
		url        string
		wantID     int64
		wantMethod string
		wantOK     bool
	}{
		{"exact alias", "Anthropic", "", 3, "alias", true},
		{"alias normalizes case+space", "  RIOT   Games ", "", 7, "alias", true},
		{"domain match when alias misses", "Anthropic PBC", "https://www.anthropic.com/careers/123", 3, "domain", true},
		{"domain strips www", "Unknown", "https://www.openai.com/jobs", 9, "domain", true},
		{"alias wins over domain when both match", "Anthropic", "https://openai.com/jobs", 3, "alias", true},
		{"no alias, no url -> miss", "Some Startup", "", 0, "none", false},
		{"no alias, unknown domain -> miss", "Some Startup", "https://example.com/x", 0, "none", false},
		{"unparseable url falls through to miss", "Some Startup", "://nope", 0, "none", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, method, ok := r.Resolve(c.company, c.url)
			if id != c.wantID || method != c.wantMethod || ok != c.wantOK {
				t.Errorf("Resolve(%q, %q) = (%d, %q, %v), want (%d, %q, %v)",
					c.company, c.url, id, method, ok, c.wantID, c.wantMethod, c.wantOK)
			}
		})
	}
}

func TestResolveReload(t *testing.T) {
	r := NewResolver(map[string]int64{}, map[string]int64{})
	if _, _, ok := r.Resolve("Anthropic", ""); ok {
		t.Fatal("expected miss before reload")
	}
	// Simulates a company added via the dashboard: the 30s reloader swaps tables.
	r.Reload(map[string]int64{"anthropic": 3}, map[string]int64{})
	if id, method, ok := r.Resolve("Anthropic", ""); !ok || id != 3 || method != "alias" {
		t.Fatalf("after reload = (%d, %q, %v), want (3, alias, true)", id, method, ok)
	}
}

func TestHostOf(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://www.anthropic.com/careers", "anthropic.com"},
		{"https://ANTHROPIC.com", "anthropic.com"},
		{"https://boards.greenhouse.io/anthropic", "boards.greenhouse.io"},
		{"https://www.example.com:8443/x", "example.com"},
		{"", ""},
		{"not-a-url", ""},
		{"://missing-scheme", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := hostOf(c.in); got != c.want {
				t.Errorf("hostOf(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
