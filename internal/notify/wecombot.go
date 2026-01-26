package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const wecombotURL = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%s"

var wecombotKeyPattern = regexp.MustCompile(`(?i)^[A-Z0-9_-]+$`)

type WeComBotTarget struct {
	key string
}

func NewWeComBotTarget(target *ParsedURL) (*WeComBotTarget, error) {
	key := target.Host
	if rawKey, ok := target.Query["key"]; ok && rawKey != "" {
		key = rawKey
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("missing key")
	}
	if !wecombotKeyPattern.MatchString(key) {
		return nil, fmt.Errorf("invalid key")
	}

	return &WeComBotTarget{key: key}, nil
}

func (w *WeComBotTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := body
	if title != "" {
		message = title + "\r\n" + body
	}

	payload := map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": message,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    fmt.Sprintf(wecombotURL, w.key),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json; charset=utf-8",
		},
		Body: string(data),
	}, nil
}

func (w *WeComBotTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := w.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(97, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"cto": map[string]any{
					"default":  4,
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
				"key": map[string]any{
					"alias_of": "key",
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
					"default":  4,
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
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{key}"},
			"tokens": map[string]any{
				"key": map[string]any{
					"map_to":   "key",
					"name":     "Bot Webhook Key",
					"private":  true,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "wecombot",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"wecombot"},
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"wecombot"},
		"service_name":     "WeCom Bot",
		"service_url":      "https://weixin.qq.com/",
		"setup_url":        "https://appriseit.com/services/wecombot/",
	})
}
