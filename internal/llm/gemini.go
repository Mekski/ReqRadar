package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Gemini calls Google's free-tier Generative Language API. We ask for JSON-only
// output via responseMimeType so the response body's text part is already valid
// JSON (the fit prompt embeds the exact shape). A structured responseSchema is a
// future hardening step — omitted for now since it can't be verified without a key.
type Gemini struct {
	key    string
	model  string
	client *http.Client
}

func NewGemini(key, model string) *Gemini {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &Gemini{key: key, model: model, client: &http.Client{Timeout: 60 * time.Second}}
}

func (g *Gemini) Model() string    { return g.model }
func (g *Gemini) Configured() bool { return g.key != "" }

// callGenerate POSTs a generateContent request body and returns the raw response
// bytes (shared by GenerateJSON and GenerateGrounded — the request/transport
// plumbing lived in both before). The API key goes in the x-goog-api-key header
// rather than the URL query string, so it can't leak into proxy/access logs.
func (g *Gemini) callGenerate(ctx context.Context, reqBody map[string]any) ([]byte, error) {
	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", g.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.key)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini status %d: %s", resp.StatusCode, truncate(raw, 300))
	}
	return raw, nil
}

func (g *Gemini) GenerateJSON(ctx context.Context, prompt string, schema map[string]any) ([]byte, error) {
	if g.key == "" {
		return nil, ErrNotConfigured
	}
	genCfg := map[string]any{
		"responseMimeType": "application/json",
		"temperature":      0.2, // low: the rubric score should be stable across runs
		"maxOutputTokens":  8192,
		// 2.5 Flash spends "thinking" tokens from the output budget, which can
		// truncate the JSON mid-structure; disable it for structured output.
		"thinkingConfig": map[string]any{"thinkingBudget": 0},
	}
	if schema != nil {
		genCfg["responseSchema"] = schema // constrains output to a valid shape
	}
	reqBody := map[string]any{
		"contents":         []any{map[string]any{"parts": []any{map[string]any{"text": prompt}}}},
		"generationConfig": genCfg,
	}
	raw, err := g.callGenerate(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse gemini response: %w", err)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		if out.PromptFeedback.BlockReason != "" {
			return nil, fmt.Errorf("gemini blocked the prompt: %s", out.PromptFeedback.BlockReason)
		}
		return nil, fmt.Errorf("gemini returned no content")
	}
	var text string
	for _, p := range out.Candidates[0].Content.Parts {
		text += p.Text
	}
	return []byte(stripFences(text)), nil
}

// stripFences removes a ```json … ``` wrapper if the model added one despite the
// JSON mime type, so the bytes parse cleanly.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

// GenerateGrounded runs the prompt with the google_search tool so the model
// answers from live web results, and returns those results' URLs from
// groundingMetadata (the actual sources searched — not model-written, so they
// can't be hallucinated). No responseMimeType: grounding + forced-JSON don't
// reliably combine, and the sentiment report is markdown anyway.
func (g *Gemini) GenerateGrounded(ctx context.Context, prompt string) (string, []Source, error) {
	if g.key == "" {
		return "", nil, ErrNotConfigured
	}
	reqBody := map[string]any{
		"contents":         []any{map[string]any{"parts": []any{map[string]any{"text": prompt}}}},
		"tools":            []any{map[string]any{"google_search": map[string]any{}}},
		"generationConfig": map[string]any{"temperature": 0.3},
	}
	raw, err := g.callGenerate(ctx, reqBody)
	if err != nil {
		return "", nil, err
	}

	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			GroundingMetadata struct {
				GroundingChunks []struct {
					Web struct {
						URI   string `json:"uri"`
						Title string `json:"title"`
					} `json:"web"`
				} `json:"groundingChunks"`
			} `json:"groundingMetadata"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", nil, fmt.Errorf("parse gemini response: %w", err)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", nil, fmt.Errorf("gemini returned no content")
	}

	var text string
	for _, p := range out.Candidates[0].Content.Parts {
		text += p.Text
	}
	var sources []Source
	seen := map[string]bool{}
	for _, ch := range out.Candidates[0].GroundingMetadata.GroundingChunks {
		if ch.Web.URI == "" || seen[ch.Web.URI] {
			continue
		}
		seen[ch.Web.URI] = true
		sources = append(sources, Source{Title: ch.Web.Title, URI: ch.Web.URI})
	}
	return text, sources, nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}
