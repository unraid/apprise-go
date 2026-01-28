package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const pushedURL = "https://api.pushed.co/1/push"

var pushedChannelRe = regexp.MustCompile(`^#?([A-Za-z0-9]+)$`)
var pushedUserRe = regexp.MustCompile(`^@([A-Za-z0-9]+)$`)

type PushedTarget struct {
	appKey    string
	appSecret string
	channels  []string
	users     []string
}

func NewPushedTarget(target *ParsedURL) (*PushedTarget, error) {
	appKey := target.Host
	segments := splitPath(target.Path)
	if appKey == "" || len(segments) == 0 {
		return nil, fmt.Errorf("missing app credentials")
	}
	appSecret := segments[0]
	targets := append([]string{}, segments[1:]...)
	if rawTargets, ok := target.Query["to"]; ok && strings.TrimSpace(rawTargets) != "" {
		targets = append(targets, parseDelimitedList(rawTargets)...)
	}

	channels, users, hasInvalid := parsePushedTargets(targets)
	if len(targets) > 0 && len(channels)+len(users) == 0 && hasInvalid {
		return nil, fmt.Errorf("no pushed targets")
	}

	return &PushedTarget{
		appKey:    appKey,
		appSecret: appSecret,
		channels:  channels,
		users:     users,
	}, nil
}

func (p *PushedTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := p.buildPayload(body, title, notifyType)
	return p.buildRequest(payload)
}

func (p *PushedTarget) buildPayload(body, title string, notifyType NotifyType) map[string]any {
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
	_ = notifyType
	return payload
}

func (p *PushedTarget) buildRequest(payload map[string]any) (RequestSpec, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}

	return RequestSpec{
		Method:  "POST",
		URL:     pushedURL,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (p *PushedTarget) Send(body, title string, notifyType NotifyType) error {
	basePayload := p.buildPayload(body, title, notifyType)
	if len(p.channels)+len(p.users) == 0 {
		spec, err := p.buildRequest(basePayload)
		if err != nil {
			return err
		}
		return SendRequest(spec)
	}

	var firstErr error
	for _, channel := range p.channels {
		payload := map[string]any{}
		for key, value := range basePayload {
			payload[key] = value
		}
		payload["target_type"] = "channel"
		payload["target_alias"] = channel
		spec, err := p.buildRequest(payload)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := SendRequest(spec); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	for _, user := range p.users {
		payload := map[string]any{}
		for key, value := range basePayload {
			payload[key] = value
		}
		payload["target_type"] = "pushed_id"
		payload["pushed_id"] = user
		spec, err := p.buildRequest(payload)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := SendRequest(spec); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func parsePushedTargets(targets []string) ([]string, []string, bool) {
	channels := []string{}
	users := []string{}
	hasInvalid := false
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if match := pushedChannelRe.FindStringSubmatch(target); len(match) > 1 {
			channels = append(channels, match[1])
			continue
		}
		if match := pushedUserRe.FindStringSubmatch(target); len(match) > 1 {
			users = append(users, match[1])
			continue
		}
		hasInvalid = true
	}
	return channels, users, hasInvalid
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
