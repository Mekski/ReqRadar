package processor

import "testing"

func TestExtractPay(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		wantOK     bool
		wantMin    float64
		wantMax    float64
		wantPeriod string
	}{
		{
			// The real Roblox phrasing (keyword BEFORE the amount, em-dash, nbsp, USD).
			name:       "greenhouse hourly, keyword before",
			content:    "<p>opportunities. Hourly Pay Range $72 &mdash; $72 USD Roles that</p>",
			wantOK:     true,
			wantMin:    72,
			wantMax:    72,
			wantPeriod: "hourly",
		},
		{
			name:       "hourly range, keyword after",
			content:    "Compensation: $45.00 - $55.00 per hour, depending on experience.",
			wantOK:     true,
			wantMin:    45,
			wantMax:    55,
			wantPeriod: "hourly",
		},
		{
			name:       "annual range with commas",
			content:    "The annual base salary range is $120,000 — $150,000.",
			wantOK:     true,
			wantMin:    120000,
			wantMax:    150000,
			wantPeriod: "annual",
		},
		{
			name:       "annual K shorthand",
			content:    "Expected salary: $120K-$150K per year.",
			wantOK:     true,
			wantMin:    120000,
			wantMax:    150000,
			wantPeriod: "annual",
		},
		{
			name:       "monthly stipend",
			content:    "Interns receive $8,000 - $9,000 per month.",
			wantOK:     true,
			wantMin:    8000,
			wantMax:    9000,
			wantPeriod: "monthly",
		},
		{
			// No period keyword near the figure -> not comp, must NOT extract.
			name:    "incidental dollar figure",
			content: "Our platform processes over $5 billion in annual transactions.",
			wantOK:  false,
		},
		{
			name:    "no money at all",
			content: "<div>Join our team to build great software. Apply now.</div>",
			wantOK:  false,
		},
		{
			// Reversed range is normalized to min<=max.
			name:       "reversed range normalized",
			content:    "Pay: $55 - $45 hourly",
			wantOK:     true,
			wantMin:    45,
			wantMax:    55,
			wantPeriod: "hourly",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pay, ok := extractPay(c.content)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (pay=%+v)", ok, c.wantOK, pay)
			}
			if !ok {
				return
			}
			if pay.Min != c.wantMin || pay.Max != c.wantMax {
				t.Errorf("range = %v-%v, want %v-%v", pay.Min, pay.Max, c.wantMin, c.wantMax)
			}
			if pay.Period != c.wantPeriod {
				t.Errorf("period = %q, want %q", pay.Period, c.wantPeriod)
			}
			if pay.Currency != "USD" {
				t.Errorf("currency = %q, want USD", pay.Currency)
			}
		})
	}
}
