package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const feishuURL = "https://open.feishu.cn/open-apis/bot/v2/hook/%s/"

var feishuTokenPattern = regexp.MustCompile(`(?i)^[A-Z0-9_-]+$`)

type FeishuTarget struct {
	token string
}

func NewFeishuTarget(target *ParsedURL) (*FeishuTarget, error) {
	token := target.Host
	if rawToken, ok := target.Query["token"]; ok && rawToken != "" {
		token = rawToken
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	if !feishuTokenPattern.MatchString(token) {
		return nil, fmt.Errorf("invalid token")
	}

	return &FeishuTarget{token: token}, nil
}

func (f *FeishuTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := body
	if title != "" {
		message = title + "\r\n" + body
	}

	payload := map[string]any{
		"msg_type": "text",
		"content": map[string]string{
			"text": message,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    fmt.Sprintf(feishuURL, f.token),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (f *FeishuTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := f.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(91, SchemaEntry{
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
				"token": map[string]any{
					"alias_of": "token",
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
			"templates": []string{"{schema}://{token}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "feishu",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"feishu"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"regex":    []string{"^[A-Z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
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
		"secure_protocols": []string{"feishu"},
		"service_name":     "Feishu",
		"service_url":      "https://open.feishu.cn/",
		"setup_url":        "https://appriseit.com/services/feishu/",
	})
}
