package notify

import (
	"encoding/json"
	"fmt"
)

const pushedURL = "https://api.pushed.co/1/push"

type PushedTarget struct {
	appKey    string
	appSecret string
}

func NewPushedTarget(target *ParsedURL) (*PushedTarget, error) {
	appKey := target.Host
	segments := splitPath(target.Path)
	if appKey == "" || len(segments) == 0 {
		return nil, fmt.Errorf("missing app credentials")
	}
	appSecret := segments[0]

	return &PushedTarget{
		appKey:    appKey,
		appSecret: appSecret,
	}, nil
}

func (p *PushedTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if title != "" {
		if body != "" {
			body = title + "\r\n" + body
		} else {
			body = title
		}
	}

	payload := map[string]any{
		"app_key":     p.appKey,
		"app_secret":  p.appSecret,
		"target_type": "app",
		"content":     body,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}

	_ = title
	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     pushedURL,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (p *PushedTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(36, SchemaEntry{
		"service_name":       "Pushed",
		"service_url":        "https://pushed.co/",
		"setup_url":          "https://appriseit.com/services/pushed/",
		"attachment_support": false,
		"category":           "native",
		"enabled":            true,
		"protocols":          []string(nil),
		"secure_protocols":   []string{"pushed"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []string{},
			"packages_required":    []string{},
		},
		"details": map[string]any{
			"args": map[string]any{
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
					"alias_of": "targets",
					"delim":    []string{",", " "},
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
				"{schema}://{app_key}/{app_secret}",
				"{schema}://{app_key}/{app_secret}@{targets}",
			},
			"tokens": map[string]any{
				"app_key": map[string]any{
					"map_to":   "app_key",
					"name":     "Application Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"app_secret": map[string]any{
					"map_to":   "app_secret",
					"name":     "Application Secret",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "pushed",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pushed"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_channel", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
			},
		},
	})
}
