package ats

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestParseDiscovery(t *testing.T) {
	cases := []struct {
		name         string
		text         string
		wantPlatform string
		wantSlug     string
	}{
		{"greenhouse", "PLATFORM: greenhouse\nSLUG: roblox", "greenhouse", "roblox"},
		{"ashby", "PLATFORM: ashby\nSLUG: notion", "ashby", "notion"},
		{"lowercase + hyphen", "platform: greenhouse\nslug: riot-games", "greenhouse", "riot-games"},
		{"markdown + mixed case", "**PLATFORM:** Greenhouse\n**SLUG:** Discord", "greenhouse", "discord"},
		{"none -> blank", "PLATFORM: none\nSLUG: -", "", ""},
		{"other ATS -> blank", "PLATFORM: lever\nSLUG: acme", "", ""},
		{"dirty slug -> blank", "PLATFORM: greenhouse\nSLUG: not a slug!", "", ""},
		{"prose -> blank", "I believe they use Workday for hiring.", "", ""},
	}
	for _, c := range cases {
		p, s := parseDiscovery(c.text)
		if p != c.wantPlatform || s != c.wantSlug {
			t.Errorf("%s: parseDiscovery = (%q,%q), want (%q,%q)", c.name, p, s, c.wantPlatform, c.wantSlug)
		}
	}
}

// rewriteTransport redirects the real ATS hosts to the test server.
type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestVerify(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"valid board with jobs", 200, `{"jobs":[{"id":1}]}`, true},
		{"valid board no openings", 200, `{"jobs":[]}`, true},
		{"404 (hallucinated slug)", 404, `not found`, false},
		{"200 but not a board", 200, `{"error":"nope"}`, false},
	}
	for _, c := range cases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		}))
		base, _ := url.Parse(ts.URL)
		svc := &Service{client: &http.Client{Transport: rewriteTransport{base}}}
		if got := svc.verify(context.Background(), "greenhouse", "x"); got != c.want {
			t.Errorf("%s: verify = %v, want %v", c.name, got, c.want)
		}
		ts.Close()
	}
}
