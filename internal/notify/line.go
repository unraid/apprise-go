package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const lineURL = "https://api.line.me/v2/bot/message/push"

var lineTargetDelimiters = regexp.MustCompile(`[ \t\r\n,#\\/]+`)

type LineTarget struct {
	token        string
	targets      []string
	includeImage bool
}

func NewLineTarget(target *ParsedURL) (*LineTarget, error) {
	targets := splitPathSegments(target.Path)

	token := target.Query["token"]
	if token == "" {
		token = target.Host
		if token != "" && !strings.HasSuffix(token, "=") {
			for i, entry := range targets {
				if strings.HasSuffix(entry, "=") {
					token = token + "/" + strings.Join(targets[:i+1], "/")
					targets = targets[i+1:]
					break
				}
			}
		}
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("missing token")
	}

	includeImage := parseBool(target.Query["image"], true)

	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range lineTargetDelimiters.Split(toValue, -1) {
			entry = strings.TrimSpace(entry)
			if entry != "" {
				targets = append(targets, entry)
			}
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &LineTarget{
		token:        token,
		targets:      targets,
		includeImage: includeImage,
	}, nil
}

func (l *LineTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(l.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	return l.buildRequestForTarget(l.targets[0], body, title, notifyType)
}

func (l *LineTarget) Send(body, title string, notifyType NotifyType) error {
	if len(l.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	for _, target := range l.targets {
		spec, err := l.buildRequestForTarget(target, body, title, notifyType)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (l *LineTarget) buildRequestForTarget(target, body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := body
	if title != "" {
		message = title + "\r\n" + body
	}

	sender := map[string]string{
		"name": "Apprise",
	}
	if l.includeImage {
		sender["iconUrl"] = appriseImageURL(notifyType, "128x128")
	}

	payload := map[string]any{
		"to": target,
		"messages": []map[string]any{
			{
				"type":   "text",
				"text":   message,
				"sender": sender,
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", l.token),
	}

	return RequestSpec{
		Method:  "POST",
		URL:     lineURL,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(39, SchemaEntry{
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
				"image": map[string]any{
					"default":  true,
					"map_to":   "include_image",
					"name":     "Include Image",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			"templates": []string{"{schema}://{token}/{targets}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "line",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"line"},
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Access Token",
					"private":  true,
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
		"secure_protocols": []string{"line"},
		"service_name":     "Line",
		"service_url":      "https://line.me/",
		"setup_url":        "https://appriseit.com/services/line/",
	})
}
