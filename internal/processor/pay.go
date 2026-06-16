package processor

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Pay is an extracted posted pay range. Comp data is posted ranges only — never
// scraped from Levels.fyi (DESIGN legal bright lines). Currency is USD (US-only).
type Pay struct {
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Period   string  `json:"period"` // "hourly" | "annual" | "monthly"
	Currency string  `json:"currency"`
}

var (
	tagRE = regexp.MustCompile(`<[^>]+>`)
	// Collapse runs of whitespace INCLUDING nbsp (\x{00a0}, common in JD HTML and
	// not matched by Go's \s) so pay phrasing reads as flat text.
	wsRE = regexp.MustCompile(`[\s\x{00a0}]+`)

	// payRangeRE matches "$72", "$72 — $72", "$45.00 - $55.00", "$120,000–$150,000",
	// "$120K-$150K". Groups: 1=first amount, 2=first K/M, 3=second amount, 4=second K/M.
	payRangeRE = regexp.MustCompile(`\$\s?([\d,]+(?:\.\d+)?)\s*([KkMm]?)\b(?:\s*(?:-|–|—|to)\s*\$?\s?([\d,]+(?:\.\d+)?)\s*([KkMm]?)\b)?`)
)

// extractPay pulls a posted pay range out of a JD's HTML content. It is
// deliberately conservative: it only returns a range when a pay-period keyword
// (hourly/annual/monthly) sits near a dollar amount, so incidental dollar figures
// in a JD ("$5B in funding", "$100 stipend for X") don't masquerade as comp. The
// period keyword may appear on either side — Greenhouse writes "Hourly Pay Range
// $72 — $72 USD" (keyword before) while other JDs write "$45/hour" (keyword after).
func extractPay(content string) (Pay, bool) {
	return extractPayText(plainText(content))
}

// extractPayText is extractPay over already-plain text, so a caller that also
// needs the plain text (the normalizers store it as jd_text) can run plainText
// once and pass the result to both instead of stripping the HTML twice.
func extractPayText(text string) (Pay, bool) {
	lower := strings.ToLower(text)

	for _, m := range payRangeRE.FindAllStringSubmatchIndex(text, -1) {
		// m[0],m[1] = full match bounds; examine a window on both sides for a period.
		start, end := m[0], m[1]
		ctxStart := max(0, start-40)
		ctxEnd := min(len(text), end+40)
		period := periodOf(lower[ctxStart:ctxEnd])
		if period == "" {
			continue
		}
		minV, ok1 := parseAmount(group(text, m, 2), group(text, m, 4))
		if !ok1 {
			continue
		}
		maxV := minV
		if g := group(text, m, 6); g != "" {
			if v, ok := parseAmount(g, group(text, m, 8)); ok {
				maxV = v
			}
		}
		if maxV < minV {
			minV, maxV = maxV, minV
		}
		// Plausibility gate: a period keyword near a dollar figure isn't enough
		// ("$5 billion ... annual revenue" would slip through), so reject amounts
		// outside a sane comp band for the detected period.
		if !plausible(minV, maxV, period) {
			continue
		}
		return Pay{Min: minV, Max: maxV, Period: period, Currency: "USD"}, true
	}
	return Pay{}, false
}

func plausible(min, max float64, period string) bool {
	switch period {
	case "hourly":
		return min >= 7 && max <= 500
	case "monthly":
		return min >= 500 && max <= 60_000
	case "annual":
		return min >= 10_000 && max <= 2_000_000
	default:
		return false
	}
}

// plainText unescapes HTML entities (&lt; &mdash; &nbsp;), drops tags, and
// collapses whitespace so pay phrasing is matchable as flat text.
func plainText(content string) string {
	s := html.UnescapeString(content)
	s = tagRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(wsRE.ReplaceAllString(s, " "))
}

// periodOf classifies a (lowercased) context window. Hourly/monthly keywords win
// over annual so "$72/hour" isn't misread when "salary" also appears nearby.
func periodOf(ctx string) string {
	switch {
	case strings.Contains(ctx, "hour") || strings.Contains(ctx, "/hr") || strings.Contains(ctx, " hr ") || strings.Contains(ctx, "hourly"):
		return "hourly"
	case strings.Contains(ctx, "month") || strings.Contains(ctx, "/mo"):
		return "monthly"
	case strings.Contains(ctx, "year") || strings.Contains(ctx, "annual") || strings.Contains(ctx, "annum") || strings.Contains(ctx, "/yr") || strings.Contains(ctx, "salary"):
		return "annual"
	default:
		return ""
	}
}

// group returns submatch i from a FindAllStringSubmatchIndex result (index pairs),
// or "" when the optional group did not participate.
func group(s string, m []int, i int) string {
	if i+1 >= len(m) || m[i] < 0 {
		return ""
	}
	return s[m[i]:m[i+1]]
}

// parseAmount turns "120,000" + "K" into a float (commas stripped, K/M applied).
func parseAmount(num, mult string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", ""), 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToUpper(mult) {
	case "K":
		v *= 1_000
	case "M":
		v *= 1_000_000
	}
	return v, true
}
