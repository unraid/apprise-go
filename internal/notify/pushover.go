package notify

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	pushoverURL              = "https://api.pushover.net/1/messages.json"
	pushoverDefaultSound     = "pushover"
	pushoverDefaultPriority  = 0
	pushoverDefaultAppDesc   = "Apprise Notifications"
	pushoverSendToAllDevices = "ALL_DEVICES"
)

var pushoverPriorityMap = map[string]int{
	"l":  -2,
	"m":  -1,
	"n":  0,
	"h":  1,
	"e":  2,
	"-2": -2,
	"-1": -1,
	"0":  0,
	"1":  1,
	"2":  2,
}

type PushoverTarget struct {
	userKey              string
	token                string
	targets              []string
	sound                string
	priority             int
	supplementalURL      string
	supplementalURLTitle string
	retry                int
	expire               int
}

func NewPushoverTarget(target *ParsedURL) (*PushoverTarget, error) {
	userKey := target.User
	token := target.Host
	if userKey == "" || token == "" {
		return nil, fmt.Errorf("missing user key or token")
	}

	targets := splitPath(target.Path)
	if rawTargets, ok := target.Query["to"]; ok && rawTargets != "" {
		targets = append(targets, splitList(rawTargets)...)
	}

	if len(targets) == 0 {
		targets = []string{pushoverSendToAllDevices}
	}

	priority := pushoverDefaultPriority
	if rawPriority := strings.TrimSpace(target.Query["priority"]); rawPriority != "" {
		priority = parsePushoverPriority(rawPriority)
	}

	retry := 0
	expire := 0
	if priority == 2 {
		retry = 900
		if rawRetry := strings.TrimSpace(target.Query["retry"]); rawRetry != "" {
			if parsed, err := strconv.Atoi(rawRetry); err == nil {
				retry = parsed
			}
		}
		if retry < 30 {
			return nil, fmt.Errorf("pushover retry must be at least 30 seconds")
		}

		expire = 3600
		if rawExpire := strings.TrimSpace(target.Query["expire"]); rawExpire != "" {
			if parsed, err := strconv.Atoi(rawExpire); err == nil {
				expire = parsed
			}
		}
		if expire < 0 || expire > 10800 {
			return nil, fmt.Errorf("pushover expire must be between 0 and 10800 seconds")
		}
	}

	sound := pushoverDefaultSound
	if rawSound := strings.TrimSpace(target.Query["sound"]); rawSound != "" {
		sound = strings.ToLower(rawSound)
	}

	return &PushoverTarget{
		userKey:              userKey,
		token:                token,
		targets:              targets,
		sound:                sound,
		priority:             priority,
		supplementalURL:      strings.TrimSpace(target.Query["url"]),
		supplementalURLTitle: strings.TrimSpace(target.Query["url_title"]),
		retry:                retry,
		expire:               expire,
	}, nil
}

func (p *PushoverTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	resolvedTitle := title
	if resolvedTitle == "" {
		resolvedTitle = pushoverDefaultAppDesc
	}

	values := url.Values{}
	values.Set("token", p.token)
	values.Set("user", p.userKey)
	values.Set("priority", fmt.Sprintf("%d", p.priority))
	values.Set("title", resolvedTitle)
	values.Set("message", body)
	values.Set("device", strings.Join(p.targets, ","))
	values.Set("sound", p.sound)
	if p.supplementalURL != "" {
		values.Set("url", p.supplementalURL)
	}
	if p.supplementalURLTitle != "" {
		values.Set("url_title", p.supplementalURLTitle)
	}
	if p.priority == 2 {
		values.Set("retry", fmt.Sprintf("%d", p.retry))
		values.Set("expire", fmt.Sprintf("%d", p.expire))
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Content-Type":  "application/x-www-form-urlencoded",
		"Authorization": basicAuthHeader(p.token, ""),
	}

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     pushoverURL,
		Headers: headers,
		Body:    values.Encode(),
	}, nil
}

func (p *PushoverTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func parsePushoverPriority(raw string) int {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	if normalized == "" {
		return pushoverDefaultPriority
	}
	if value, err := strconv.Atoi(normalized); err == nil {
		if value >= -2 && value <= 2 {
			return value
		}
		return pushoverDefaultPriority
	}
	for key, value := range pushoverPriorityMap {
		if strings.HasPrefix(normalized, key) {
			return value
		}
	}
	return pushoverDefaultPriority
}

func init() {
	RegisterSchemaEntryOrdered(35, SchemaEntry{
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
				"expire": map[string]any{
					"default":  3600,
					"map_to":   "expire",
					"max":      10800,
					"min":      0,
					"name":     "Expire",
					"private":  false,
					"required": false,
					"type":     "int",
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
				"priority": map[string]any{
					"default":  0,
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{-2, -1, 0, 1, 2},
				},
				"retry": map[string]any{
					"default":  900,
					"map_to":   "retry",
					"min":      30,
					"name":     "Retry",
					"private":  false,
					"required": false,
					"type":     "int",
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
					"default":  "pushover",
					"map_to":   "sound",
					"name":     "Sound",
					"private":  false,
					"regex":    []string{"^[a-z]{1,12}$", "i"},
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
				"url": map[string]any{
					"map_to":   "supplemental_url",
					"name":     "URL",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"url_title": map[string]any{
					"map_to":   "supplemental_url_title",
					"name":     "URL Title",
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
			"templates": []string{"{schema}://{user_key}@{token}", "{schema}://{user_key}@{token}/{targets}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "pover",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pover"},
				},
				"target_device": map[string]any{
					"map_to":   "targets",
					"name":     "Target Device",
					"private":  false,
					"regex":    []string{"^[a-z0-9_-]{1,25}$", "i"},
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_device"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Access Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"user_key": map[string]any{
					"map_to":   "user_key",
					"name":     "User Key",
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
		"secure_protocols": []string{"pover"},
		"service_name":     "Pushover",
		"service_url":      "https://pushover.net/",
		"setup_url":        "https://appriseit.com/services/pushover/",
	})
}
