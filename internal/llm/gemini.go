package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func (g *Gemini) GenerateJSON(ctx context.Context, prompt string) ([]byte, error) {
	if g.key == "" {
		return nil, ErrNotConfigured
	}
	reqBody := map[string]any{
		"contents": []any{
			map[string]any{"parts": []any{map[string]any{"text": prompt}}},
		},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
			"temperature":      0.2, // low: the rubric score should be stable across runs
		},
	}
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.model, g.key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini status %d: %s", resp.StatusCode, truncate(raw, 300))
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
	return []byte(out.Candidates[0].Content.Parts[0].Text), nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}
