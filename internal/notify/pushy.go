package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const pushyURL = "https://api.pushy.me/push"

type PushyTarget struct {
	apiKey string
	target string
	sound  string
	badge  *int
}

func NewPushyTarget(target *ParsedURL) (*PushyTarget, error) {
	apiKey := target.Host
	if rawKey, ok := target.Query["key"]; ok && rawKey != "" {
		apiKey = rawKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	targets := splitPath(target.Path)
	if rawTargets, ok := target.Query["to"]; ok && rawTargets != "" {
		targets = append(targets, splitList(rawTargets)...)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	selected := ""
	for _, entry := range targets {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "@") && len(trimmed) > 1 {
			selected = trimmed[1:]
			break
		}
		if strings.HasPrefix(trimmed, "#") && len(trimmed) > 1 {
			selected = trimmed[1:]
			break
		}
		if isAlnum(trimmed) {
			selected = trimmed
			break
		}
	}
	if selected == "" {
		return nil, fmt.Errorf("no valid targets")
	}

	sound := ""
	if rawSound, ok := target.Query["sound"]; ok && rawSound != "" {
		sound = rawSound
	}

	var badge *int
	if rawBadge, ok := target.Query["badge"]; ok && rawBadge != "" {
		if value, err := strconv.Atoi(strings.TrimSpace(rawBadge)); err == nil && value >= 0 {
			badge = &value
		}
	}

	return &PushyTarget{
		apiKey: apiKey,
		target: selected,
		sound:  sound,
		badge:  badge,
	}, nil
}

func (p *PushyTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"to": p.target,
		"data": map[string]any{
			"message": body,
		},
		"notification": map[string]any{
			"body": body,
		},
	}

	if title != "" {
		payload["notification"].(map[string]any)["title"] = title
	}
	if p.sound != "" {
		payload["notification"].(map[string]any)["sound"] = p.sound
	}
	if p.badge != nil {
		payload["notification"].(map[string]any)["badge"] = *p.badge
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	u, err := url.Parse(pushyURL)
	if err != nil {
		return RequestSpec{}, err
	}
	q := url.Values{}
	q.Set("api_key", p.apiKey)
	u.RawQuery = q.Encode()

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Accepts":      "application/json",
		"Content-Type": "application/json",
	}

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     u.String(),
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (p *PushyTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func isAlnum(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return value != ""
}

func init() {
	RegisterSchemaEntryOrdered(81, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"badge": map[string]any{
					"map_to":   "badge",
					"min":      0,
					"name":     "Badge",
					"private":  false,
					"required": false,
					"type":     "int",
				},
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
					"alias_of": "apikey",
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
				"sound": map[string]any{
					"map_to":   "sound",
					"name":     "Sound",
					"private":  false,
					"required": false,
					"type":     "string",
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
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "Secret API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "pushy",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pushy"},
				},
				"target_device": map[string]any{
					"map_to":   "targets",
					"name":     "Target Device",
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_topic": map[string]any{
					"map_to":   "targets",
					"name":     "Target Topic",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_device", "target_topic"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
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
		"secure_protocols": []string{"pushy"},
		"service_name":     "Pushy",
		"service_url":      "https://pushy.me/",
		"setup_url":        "https://appriseit.com/services/pushy/",
	})
}
