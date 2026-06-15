package processor

import (
	"reflect"
	"testing"
)

func TestNormalizeSimplify(t *testing.T) {
	// A real listings.json entry shape (extra wire fields like source/is_visible
	// are intentionally present to prove the normalizer ignores unknown fields).
	payload := []byte(`{
		"source": "Simplify",
		"company_name": "Pega",
		"title": "Software Engineer Summer Intern",
		"url": "https://www.pega.com/careers/22692",
		"locations": ["Waltham, MA"],
		"terms": ["Summer 2026"],
		"category": "Software Engineering",
		"is_visible": true,
		"date_posted": 1761004443
	}`)

	got, err := normalizeSimplify(payload)
	if err != nil {
		t.Fatalf("normalizeSimplify: %v", err)
	}
	want := Posting{
		Company:   "Pega",
		Title:     "Software Engineer Summer Intern",
		URL:       "https://www.pega.com/careers/22692",
		Locations: []string{"Waltham, MA"},
		Terms:     []string{"Summer 2026"},
		Category:  "Software Engineering",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizeSimplify =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestNormalizeSimplifyBadJSON(t *testing.T) {
	if _, err := normalizeSimplify([]byte("{not json")); err == nil {
		t.Error("expected error on malformed payload, got nil")
	}
}
