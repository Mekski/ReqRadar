package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// rewriteTransport redirects requests to a test server, regardless of the
// hardcoded api.telegram.org host SendMessage builds — so the real request
// construction and response handling run against httptest.
type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	c := New("test-token")
	c.http = &http.Client{Transport: rewriteTransport{base}}
	return c
}

func TestSendMessageSuccess(t *testing.T) {
	var gotForm url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	if err := newTestClient(t, ts).SendMessage(context.Background(), "chat-1", "hello"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	// Request shape: chat_id, text, and web-page preview disabled.
	if gotForm.Get("chat_id") != "chat-1" {
		t.Errorf("chat_id = %q", gotForm.Get("chat_id"))
	}
	if gotForm.Get("text") != "hello" {
		t.Errorf("text = %q", gotForm.Get("text"))
	}
	if gotForm.Get("disable_web_page_preview") != "true" {
		t.Errorf("disable_web_page_preview = %q, want true", gotForm.Get("disable_web_page_preview"))
	}
}

func TestSendMessageAPIError(t *testing.T) {
	// HTTP 200 but ok:false (e.g. bad chat_id) — surface the description.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
	}))
	defer ts.Close()

	err := newTestClient(t, ts).SendMessage(context.Background(), "bad", "hi")
	if err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("err = %v, want it to mention 'chat not found'", err)
	}
}

func TestSendMessageRateLimited(t *testing.T) {
	// 429 with parameters.retry_after — the wait must be surfaced (OBS-1).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":30}}`))
	}))
	defer ts.Close()

	err := newTestClient(t, ts).SendMessage(context.Background(), "chat-1", "hi")
	if err == nil {
		t.Fatal("expected error on 429")
	}
	for _, want := range []string{"429", "retry_after 30"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to contain %q", err.Error(), want)
		}
	}
}

func TestSendMessageServerErrorNonJSON(t *testing.T) {
	// A 5xx with a non-JSON body must still produce a useful error (the prior
	// code returned an empty "failed: " here).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer ts.Close()

	err := newTestClient(t, ts).SendMessage(context.Background(), "chat-1", "hi")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v, want it to mention HTTP 502", err)
	}
}
