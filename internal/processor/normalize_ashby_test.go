package processor

import (
	"encoding/json"
	"testing"
)

func TestNormalizeAshby(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"org":         "notion",
		"title":       "Software Engineer Intern (Fall 2026)",
		"url":         "https://jobs.ashbyhq.com/notion/abc",
		"location":    "San Francisco, California",
		"department":  "Engineering",
		"description": "<p>The hourly pay range for this role is $57/hr for Bachelors students.</p>",
	})
	p, err := normalizeAshby(payload)
	if err != nil {
		t.Fatalf("normalizeAshby: %v", err)
	}
	if p.Company != "notion" { // resolves via org slug (Ashby exposes no company name)
		t.Errorf("company = %q, want org slug", p.Company)
	}
	if p.Category != "Software Engineering" {
		t.Errorf("category = %q", p.Category)
	}
	if len(p.Terms) != 1 || p.Terms[0] != "Fall 2026" {
		t.Errorf("terms = %v, want [Fall 2026]", p.Terms)
	}
	if p.PayPeriod != "hourly" || p.PayMin != 57 {
		t.Errorf("pay = %v %v %q, want 57 hourly", p.PayMin, p.PayMax, p.PayPeriod)
	}
}
