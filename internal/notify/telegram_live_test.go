package notify_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTelegramLiveFormattingAgainstBotAPI(t *testing.T) {
	token := strings.TrimSpace(os.Getenv("APPRISE_GO_TELEGRAM_BOT_TOKEN"))
	if token == "" {
		t.Skip("set APPRISE_GO_TELEGRAM_BOT_TOKEN to validate Telegram formatting against the live Bot API")
	}

	chatID := telegramLiveDestination(t)
	generalizedMarkdown := strings.Join([]string{
		"# Deploy Summary",
		"",
		"**Bold** _Italics_ ~~Strike~~ [Docs](https://example.com/docs)",
		"",
		"`inline code`",
		"",
		"```go",
		"if x > 0 { return `tick` }\\path",
		"```",
		"",
		"- first",
		"- second",
		"",
		"<em>inline html</em>",
	}, "\n")
	cases := []struct {
		name       string
		rawURL     string
		body       string
		bodyFormat string
	}{
		{
			name:       "markdown v1 fenced code",
			rawURL:     "tgram://123456:abcdef/7890/?format=markdown&mdv=v1",
			body:       "**Bold**\n_Italics_\n```\ncode\n```",
			bodyFormat: "markdown",
		},
		{
			name:       "markdown v2 fenced code",
			rawURL:     "tgram://123456:abcdef/7890/?format=markdown&mdv=v2",
			body:       "```go\nif x > 0 { return `tick` }\\path\n```",
			bodyFormat: "markdown",
		},
		{
			name:       "telegram html fenced code",
			rawURL:     "tgram://123456:abcdef/7890/?format=html",
			body:       "**Bold**\n_Italics_\n```\ncode\n```",
			bodyFormat: "markdown",
		},
		{
			name:       "html input to markdown v1",
			rawURL:     "tgram://123456:abcdef/7890/?format=markdown&mdv=v1",
			body:       "<b>Bold</b> <i>Italics</i> Text",
			bodyFormat: "html",
		},
		{
			name:       "html input to markdown v2",
			rawURL:     "tgram://123456:abcdef/7890/?format=markdown&mdv=v2",
			body:       "<b>Bold</b> <i>Italics</i> Text",
			bodyFormat: "html",
		},
		{
			name:       "generalized markdown corpus to markdown v1",
			rawURL:     "tgram://123456:abcdef/7890/?format=markdown&mdv=v1",
			body:       generalizedMarkdown,
			bodyFormat: "markdown",
		},
		{
			name:       "generalized markdown corpus to markdown v2",
			rawURL:     "tgram://123456:abcdef/7890/?format=markdown&mdv=v2",
			body:       generalizedMarkdown,
			bodyFormat: "markdown",
		},
		{
			name:       "generalized markdown corpus to html",
			rawURL:     "tgram://123456:abcdef/7890/?format=html",
			body:       generalizedMarkdown,
			bodyFormat: "markdown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := captureTelegramPayload(t, tc.rawURL, tc.body, "", tc.bodyFormat)
			payload["chat_id"] = chatID
			telegramLiveAssertParseable(t, token, payload)
		})
	}
}

func telegramLiveDestination(t *testing.T) any {
	t.Helper()

	if chatID := strings.TrimSpace(os.Getenv("APPRISE_GO_TELEGRAM_CHAT_ID")); chatID != "" {
		return chatID
	}

	t.Skip("set APPRISE_GO_TELEGRAM_CHAT_ID to run Telegram live validation against an explicit destination")
	return nil
}

func telegramLiveAssertParseable(t *testing.T, token string, payload map[string]any) {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal telegram payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new telegram request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := telegramLiveClient().Do(req)
	if err != nil {
		t.Fatalf("telegram sendMessage: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode telegram sendMessage: %v", err)
	}
	if strings.Contains(result.Description, "can't parse entities") {
		t.Fatalf("telegram rejected formatting: status=%d description=%q payload=%q", resp.StatusCode, result.Description, payload["text"])
	}
	if !result.OK {
		t.Fatalf("telegram rejected delivered message: status=%d code=%d description=%q", resp.StatusCode, result.ErrorCode, result.Description)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		t.Fatalf("telegram rejected request before parse validation completed: status=%d code=%d description=%q", resp.StatusCode, result.ErrorCode, result.Description)
	}
}

func telegramLiveClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}
