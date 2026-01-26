package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const pushbulletURL = "https://api.pushbullet.com/v2/pushes"

type PushbulletTarget struct {
	accessToken string
	target      string
}

func NewPushbulletTarget(target *ParsedURL) (*PushbulletTarget, error) {
	accessToken := target.Host
	if accessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}

	targets := splitPath(target.Path)
	if rawTargets, ok := target.Query["to"]; ok && rawTargets != "" {
		targets = append(targets, splitList(rawTargets)...)
	}

	selected := ""
	for _, entry := range targets {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		selected = trimmed
		break
	}

	return &PushbulletTarget{
		accessToken: accessToken,
		target:      selected,
	}, nil
}

func (p *PushbulletTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"type":  "note",
		"title": title,
		"body":  body,
	}

	if p.target != "" {
		switch {
		case strings.HasPrefix(p.target, "#") && len(p.target) > 1:
			payload["channel_tag"] = p.target[1:]
		case looksLikeEmail(p.target):
			payload["email"] = p.target
		default:
			payload["device_iden"] = p.target
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": basicAuthHeader(p.accessToken, ""),
	}

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     pushbulletURL,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (p *PushbulletTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func looksLikeEmail(value string) bool {
	at := strings.Index(value, "@")
	if at <= 0 || at == len(value)-1 {
		return false
	}
	return strings.Contains(value[at+1:], ".")
}

func init() {
	RegisterSchemaEntryOrdered(113, SchemaEntry{
		"attachment_support": true,
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
			"templates": []string{"{schema}://{accesstoken}", "{schema}://{accesstoken}/{targets}"},
			"tokens": map[string]any{
				"accesstoken": map[string]any{
					"map_to":   "accesstoken",
					"name":     "Access Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "pbul",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pbul"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_device": map[string]any{
					"map_to":   "targets",
					"name":     "Target Device",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_email": map[string]any{
					"map_to":   "targets",
					"name":     "Target Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_channel", "target_device", "target_email"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
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
		"secure_protocols": []string{"pbul"},
		"service_name":     "Pushbullet",
		"service_url":      "https://www.pushbullet.com/",
		"setup_url":        "https://appriseit.com/services/pushbullet/",
	})
}
