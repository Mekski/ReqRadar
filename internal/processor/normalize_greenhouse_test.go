package processor

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeGreenhouse(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"org":          "roblox",
		"title":        "Software Engineer Intern, Summer 2027",
		"company_name": "Roblox",
		"url":          "https://careers.roblox.com/jobs/1",
		"locations":    []string{"San Mateo, CA"},
		"departments":  []string{"Early Career Internships"},
	})
	p, err := normalizeGreenhouse(payload)
	if err != nil {
		t.Fatalf("normalizeGreenhouse: %v", err)
	}
	if p.Company != "Roblox" {
		t.Errorf("company = %q", p.Company)
	}
	if p.Category != "Software Engineering" {
		t.Errorf("category = %q, want Software Engineering", p.Category)
	}
	if !reflect.DeepEqual(p.Terms, []string{"Summer 2027"}) {
		t.Errorf("terms = %v, want [Summer 2027]", p.Terms)
	}
}

// Company falls back to the org slug (seeded as an alias) when company_name is blank.
func TestNormalizeGreenhouseCompanyFallback(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"org": "xai", "title": "ML Intern", "company_name": ""})
	p, err := normalizeGreenhouse(payload)
	if err != nil {
		t.Fatalf("normalizeGreenhouse: %v", err)
	}
	if p.Company != "xai" {
		t.Errorf("company = %q, want the org slug fallback", p.Company)
	}
}

func TestTermsFromTitle(t *testing.T) {
	cases := []struct {
		title string
		want  []string
	}{
		{"Software Engineer Intern, Summer 2027", []string{"Summer 2027"}},
		{"SWE Intern - Fall 2026", []string{"Fall 2026"}},
		{"SUMMER Internship", []string{"Summer"}},
		{"PhD Intern", nil}, // no season named -> not counted toward summer seasonality
	}
	for _, c := range cases {
		if got := termsFromTitle(c.title); !reflect.DeepEqual(got, c.want) {
			t.Errorf("termsFromTitle(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}

func TestInferCategory(t *testing.T) {
	cases := []struct {
		title string
		dept  []string
		want  string
	}{
		{"Software Engineer Intern", nil, "Software Engineering"},
		{"Backend Intern", []string{"Software Engineering"}, "Software Engineering"},
		{"Applied Scientist - PhD Intern", []string{"Early Career Internships"}, "AI/ML/Data"},
		{"Data Scientist Intern", nil, "AI/ML/Data"},
		{"Product Design Intern", []string{"Design"}, ""},
	}
	for _, c := range cases {
		if got := inferCategory(c.title, c.dept); got != c.want {
			t.Errorf("inferCategory(%q, %v) = %q, want %q", c.title, c.dept, got, c.want)
		}
	}
}
