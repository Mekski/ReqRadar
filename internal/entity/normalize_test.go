package entity

import "testing"

func TestNormalize(t *testing.T) {
	// Normalize is the single definition shared by the seeder and the resolver,
	// so an alias seeded as "riot games" matches a raw "Riot  Games" from a feed.
	// It currently lowercases and collapses internal whitespace only; suffix and
	// punctuation stripping are deliberately deferred (see normalize.go).
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already canonical", "anthropic", "anthropic"},
		{"uppercase", "ANTHROPIC", "anthropic"},
		{"mixed case", "Riot Games", "riot games"},
		{"leading/trailing space", "  Anthropic  ", "anthropic"},
		{"collapse internal runs", "Riot    Games", "riot games"},
		{"tabs and newlines are whitespace", "Google\tLLC\n", "google llc"},
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		// Documents current behavior: punctuation is NOT stripped yet, so a seeded
		// alias must match the same punctuation. This guards against a silent change.
		{"punctuation preserved", "Ben & Jerry's", "ben & jerry's"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Normalize(c.in); got != c.want {
				t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
