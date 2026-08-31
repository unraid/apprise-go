package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const lautherURL = "https://api.lauther.id/v1/push"

var lautherTokenRe = regexp.MustCompile(`(?i)^lpt_[a-z0-9]+$`)

var lautherPriorityNames = map[int]string{
	-2: "lowest",
	-1: "low",
	0:  "normal",
	1:  "high",
	2:  "emergency",
}

// Keep the longer names before their prefixes. In particular, "lowest" must
// not be interpreted as the shorter "low" priority.
var lautherPriorityPrefixes = []struct {
	name  string
	value int
}{
	{name: "emergency", value: 2},
	{name: "lowest", value: -2},
	{name: "normal", value: 0},
	{name: "high", value: 1},
	{name: "low", value: -1},
}

type LautherTarget struct {
	token    string
	priority int
	sound    string
	click    string
	icon     string
	color    string
	group    string
	route    string
}

func NewLautherTarget(target *ParsedURL) (*LautherTarget, error) {
	token := strings.TrimSpace(target.Query["token"])
	if token == "" {
		token = strings.TrimSpace(target.Host)
	}
	if !lautherTokenRe.MatchString(token) {
		return nil, fmt.Errorf("invalid Lauther token: %s", token)
	}

	priority, err := parseLautherPriority(target.Query["priority"])
	if err != nil {
		return nil, err
	}

	return &LautherTarget{
		token:    token,
		priority: priority,
		sound:    strings.TrimSpace(target.Query["sound"]),
		click:    strings.TrimSpace(target.Query["click"]),
		icon:     strings.TrimSpace(target.Query["icon"]),
		color:    strings.TrimSpace(target.Query["color"]),
		group:    strings.TrimSpace(target.Query["group"]),
		route:    strings.TrimSpace(target.Query["route"]),
	}, nil
}

func parseLautherPriority(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	lower := strings.ToLower(raw)
	for _, prefix := range lautherPriorityPrefixes {
		if strings.HasPrefix(lower, prefix.name) {
			return prefix.value, nil
		}
	}

	priority, err := strconv.Atoi(raw)
	if err != nil {
		// Upstream falls back to normal for a non-numeric, non-name value.
		return 0, nil
	}
	if _, ok := lautherPriorityNames[priority]; !ok {
		return 0, fmt.Errorf("invalid Lauther priority: %s", raw)
	}
	return priority, nil
}

func (l *LautherTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	_ = notifyType

	payload := map[string]any{
		"title":    title,
		"message":  body,
		"priority": l.priority,
	}
	if l.sound != "" {
		payload["sound"] = l.sound
	}
	if l.click != "" {
		payload["url"] = l.click
	}
	if l.icon != "" {
		payload["icon"] = l.icon
	}
	if l.color != "" {
		payload["color"] = l.color
	}
	if l.group != "" {
		payload["tag"] = l.group
	}
	if l.route != "" {
		payload["path"] = l.route
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    lautherURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + l.token,
		},
		Body: string(data),
	}, nil
}

func (l *LautherTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := l.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

// Lauther has no attachment API. Upstream drops attachments when a body is
// present, while attachment-only notifications are skipped as unsupported.
func (l *LautherTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	if len(attachments) > 0 && body == "" {
		return ErrAttachmentsUnsupported
	}
	return l.Send(body, title, notifyType)
}

func init() {
	RegisterSchemaEntryOrdered(167, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"click": map[string]any{
					"map_to":   "click",
					"name":     "Click URL",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"color": map[string]any{
					"map_to":   "color",
					"name":     "Color",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"group": map[string]any{
					"map_to":   "group",
					"name":     "Group",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"icon": map[string]any{
					"map_to":   "icon",
					"name":     "Icon URL",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"optional": map[string]any{
					"default":  false,
					"map_to":   "optional",
					"name":     "Optional Service",
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
				"priority": map[string]any{
					"default":  0,
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []int{-2, -1, 0, 1, 2},
				},
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"retry": map[string]any{
					"default":  0,
					"map_to":   "retry",
					"max":      10,
					"min":      0,
					"name":     "Service Retry",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"route": map[string]any{
					"map_to":   "route",
					"name":     "Route",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"rto": map[string]any{
					"default":  4.0,
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
				"wait": map[string]any{
					"default":  0.0,
					"map_to":   "wait",
					"max":      20.0,
					"min":      0.0,
					"name":     "Inter-Retry Wait",
					"private":  false,
					"required": false,
					"type":     "float",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "lauther",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"lauther"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"regex":    []string{"^lpt_[a-z0-9]+$", "i"},
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
		"secure_protocols": []string{"lauther"},
		"service_name":     "Lauther",
		"service_url":      "https://lauther.app/",
		"setup_url":        "https://appriseit.com/services/lauther/",
	})
}
