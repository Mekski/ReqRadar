// Package telegram is a minimal Telegram Bot API client — just enough to send
// alert messages. The bot is @ReqRadarBot; the token comes from TELEGRAM_BOT_TOKEN.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	token string
	http  *http.Client
}

func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 10 * time.Second}}
}

// SendMessage sends text to a chat. Web-page previews are disabled so a posting
// URL doesn't expand into a large card.
func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.token)
	form := url.Values{
		"chat_id":                  {chatID},
		"text":                     {text},
		"disable_web_page_preview": {"true"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var body struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if !body.OK {
		return fmt.Errorf("telegram sendMessage failed: %s", body.Description)
	}
	return nil
}
