package expected

import "testing"

func TestParseAnswer(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		wantMonth string
		wantOK    bool
	}{
		{"month with why", "ANSWER: Sep\nWHY: Riot opens in September per its careers page.", "Sep", true},
		{"full month name", "ANSWER: September", "Sep", true},
		{"lowercase", "answer: oct", "Oct", true},
		{"trailing punctuation", "ANSWER: Aug.", "Aug", true},
		{"markdown bold", "**ANSWER:** Jul", "Jul", true},
		{"rolling", "ANSWER: rolling", "rolling", true},
		{"rolling phrase", "ANSWER: rolling (year-round)", "rolling", true},
		{"unknown -> blank", "ANSWER: unknown", "", false},
		{"garbage -> blank", "I could not determine this.", "", false},
		{"empty answer -> blank", "ANSWER:\nWHY: nothing found", "", false},
	}
	for _, c := range cases {
		gotMonth, gotOK := parseAnswer(c.text)
		if gotMonth != c.wantMonth || gotOK != c.wantOK {
			t.Errorf("%s: parseAnswer = (%q, %v), want (%q, %v)", c.name, gotMonth, gotOK, c.wantMonth, c.wantOK)
		}
	}
}
