package notify

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const wpushURL = "https://api.wpush.cn/api/v1/send"

const (
	wpushChannelWeChat     = "wechat"
	wpushChannelApp        = "app"
	wpushChannelSMS        = "sms"
	wpushChannelMail       = "mail"
	wpushChannelWebhook    = "webhook"
	wpushChannelDingTalk   = "dingtalk"
	wpushChannelFeishu     = "feishu"
	wpushChannelWeChatWork = "wechat_work"
	wpushChannelClawBot    = "clawbot"
	wpushChannelQQBot      = "qqbot"

	wpushDefaultChannel = wpushChannelWeChat
)

var wpushChannels = map[string]struct{}{
	wpushChannelWeChat:     {},
	wpushChannelApp:        {},
	wpushChannelSMS:        {},
	wpushChannelMail:       {},
	wpushChannelWebhook:    {},
	wpushChannelDingTalk:   {},
	wpushChannelFeishu:     {},
	wpushChannelWeChatWork: {},
	wpushChannelClawBot:    {},
	wpushChannelQQBot:      {},
}

// wpushAPIKeyRe mirrors upstream Apprise: keys start with WPUSH.
var wpushAPIKeyRe = regexp.MustCompile(`(?i)^WPUSH[a-z0-9]+$`)

type WPushTarget struct {
	apiKey    string
	channel   string
	topicCode string
}

func NewWPushTarget(target *ParsedURL) (*WPushTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if raw, ok := target.Query["apikey"]; ok && strings.TrimSpace(raw) != "" {
		apiKey = strings.TrimSpace(raw)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}
	if !wpushAPIKeyRe.MatchString(apiKey) {
		return nil, fmt.Errorf("invalid wpush apikey")
	}

	channel := wpushDefaultChannel
	if raw := strings.TrimSpace(target.Query["channel"]); raw != "" {
		candidate := strings.ToLower(raw)
		if _, ok := wpushChannels[candidate]; !ok {
			return nil, fmt.Errorf("invalid wpush channel: %q", raw)
		}
		channel = candidate
	}

	topicCode := strings.TrimSpace(target.Query["topic_code"])
	if topicCode == "" {
		topicCode = strings.TrimSpace(target.Query["to"])
	}

	return &WPushTarget{
		apiKey:    apiKey,
		channel:   channel,
		topicCode: topicCode,
	}, nil
}

type wpushPayload struct {
	APIKey    string `json:"apikey"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Channel   string `json:"channel"`
	TopicCode string `json:"topic_code,omitempty"`
}

func (w *WPushTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	_ = notifyType

	resolvedTitle := title
	if resolvedTitle == "" {
		resolvedTitle = body
	}

	payload := wpushPayload{
		APIKey:  w.apiKey,
		Title:   resolvedTitle,
		Content: body,
		Channel: w.channel,
	}
	if w.topicCode != "" {
		payload.TopicCode = w.topicCode
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    wpushURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (w *WPushTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := w.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	req, err := spec.HTTPRequest()
	if err != nil {
		return err
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// WPUSH defines success as JSON code === 0. Decode the body first so a
	// non-2xx response that still carries code:0 is not rejected by the
	// generic HTTP-status helper.
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return &HTTPStatusError{StatusCode: resp.StatusCode}
		}
		return fmt.Errorf("wpush invalid json response: %w", err)
	}
	if response.Code != 0 {
		msg := response.Message
		if msg == "" {
			msg = "Unknown error"
		}
		return fmt.Errorf("wpush api code=%d: %s", response.Code, msg)
	}

	return nil
}

func init() {
	RegisterSchemaEntryOrdered(169, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"apikey": map[string]any{
					"alias_of": "apikey",
				},
				"channel": map[string]any{
					"default":  wpushDefaultChannel,
					"map_to":   "channel",
					"name":     "Channel",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values": []string{
						wpushChannelWeChat,
						wpushChannelApp,
						wpushChannelSMS,
						wpushChannelMail,
						wpushChannelWebhook,
						wpushChannelDingTalk,
						wpushChannelFeishu,
						wpushChannelWeChatWork,
						wpushChannelClawBot,
						wpushChannelQQBot,
					},
				},
				"cto": map[string]any{
					"default":  4.0,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"emojis": map[string]any{
					"default":  false,
					"map_to":   "emojis",
					"name":     "Interpret Emojis",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"format": map[string]any{
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"overflow": map[string]any{
					"default":  "upstream",
					"map_to":   "overflow",
					"name":     "Overflow Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"split", "truncate", "upstream"},
				},
				"rto": map[string]any{
					"default":  4.0,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"to": map[string]any{
					"alias_of": "topic_code",
				},
				"topic_code": map[string]any{
					"map_to":   "topic_code",
					"name":     "Topic Code",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"verify": map[string]any{
					"default":  true,
					"map_to":   "verify",
					"name":     "Verify SSL",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
			},
			"kwargs": map[string]any{},
			"templates": []string{
				"{schema}://{apikey}",
			},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^WPUSH[a-z0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "wpush",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"wpush"},
				},
			},
		},
		"enabled":   true,
		"protocols": []string(nil),
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []string{},
			"packages_required":    []string{},
		},
		"secure_protocols": []string{"wpush"},
		"service_name":     "WPUSH",
		"service_url":      "https://wpush.cn/",
		"setup_url":        "https://wpush.cn/docs",
	})
}
