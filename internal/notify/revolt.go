package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

type RevoltTarget struct {
	botToken string
	targets  []string
	iconURL  string
	link     string
	format   string
}

func NewRevoltTarget(target *ParsedURL) (*RevoltTarget, error) {
	botToken := strings.TrimSpace(target.Host)
	if botToken == "" {
		return nil, fmt.Errorf("missing bot token")
	}

	targets := splitPath(target.Path)
	if toValue, ok := target.Query["to"]; ok && strings.TrimSpace(toValue) != "" {
		targets = append(targets, parseDelimitedList(toValue)...)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	format := normalizeNotifyFormat(target.Query["format"])
	if format == "" {
		format = "markdown"
	}

	return &RevoltTarget{
		botToken: botToken,
		targets:  targets,
		iconURL:  strings.TrimSpace(target.Query["icon_url"]),
		link:     strings.TrimSpace(target.Query["url"]),
		format:   format,
	}, nil
}

func (r *RevoltTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := r.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (r *RevoltTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(r.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	iconURL := r.iconURL
	if iconURL == "" {
		iconURL = appriseImageURL(notifyType, "256x256")
	}

	content := body
	if strings.TrimSpace(title) != "" {
		content = title + "\n" + body
	}
	payload := map[string]any{
		"content": content,
	}
	if r.format == "markdown" {
		embed := map[string]any{
			"title":       nil,
			"description": body,
			"colour":      appriseColor(notifyType),
			"replies":     nil,
		}
		if strings.TrimSpace(title) != "" {
			if len(title) > 100 {
				title = title[:100]
			}
			embed["title"] = title
		}
		if iconURL != "" {
			embed["icon_url"] = iconURL
		}
		if r.link != "" {
			embed["url"] = r.link
		}
		payload = map[string]any{
			"embeds": []any{embed},
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	url := fmt.Sprintf("https://api.revolt.chat/channels/%s/messages", r.targets[0])

	return RequestSpec{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "application/json; charset=utf-8",
			"Content-Type": "application/json; charset=utf-8",
			"X-Bot-Token":  r.botToken,
		},
		Body: string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(3, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"bot_token": map[string]any{
					"alias_of": "bot_token",
				},
				"channel": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
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
				"icon_url": map[string]any{
					"map_to":   "icon_url",
					"name":     "Icon URL",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"url": map[string]any{
					"map_to":   "link",
					"name":     "Embed URL",
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
			"templates": []string{"{schema}://{bot_token}/{targets}"},
			"tokens": map[string]any{
				"bot_token": map[string]any{
					"map_to":   "bot_token",
					"name":     "Bot Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "revolt",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"revolt"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Channel ID",
					"private":  true,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_channel"},
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
		"secure_protocols": []string{"revolt"},
		"service_name":     "Revolt",
		"service_url":      "https://revolt.chat/",
		"setup_url":        "https://appriseit.com/services/revolt/",
	})
}
