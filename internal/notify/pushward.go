package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const pushwardURL = "https://api.pushward.app/notifications"

// pushwardAPIKey matches the key format the service issues.
var pushwardAPIKey = regexp.MustCompile(`(?i)^hlk_[A-Za-z0-9]+$`)

// pushwardLevels are the delivery levels, longest-prefix matched so short
// forms such as "crit" resolve.
var pushwardLevels = []string{"passive", "active", "time-sensitive", "critical"}

// pushwardDefaultLevels maps a notification type onto its level. Critical is
// never a default: it bypasses the device's silent mode and must be opted into.
var pushwardDefaultLevels = map[NotifyType]string{
	NotifyInfo:    "active",
	NotifySuccess: "active",
	NotifyWarning: "time-sensitive",
	NotifyFailure: "time-sensitive",
}

type PushWardTarget struct {
	apikey   string
	level    string
	levelMap map[NotifyType]string
	volume   *float64
}

// resolvePushWardLevel matches a full or abbreviated level name.
func resolvePushWardLevel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	for _, level := range pushwardLevels {
		if strings.HasPrefix(level, value) {
			return level
		}
	}
	return ""
}

func NewPushWardTarget(target *ParsedURL) (*PushWardTarget, error) {
	apikey := strings.TrimSpace(target.Query["apikey"])
	if apikey == "" {
		apikey = strings.TrimSpace(target.Host)
	}
	if !pushwardAPIKey.MatchString(apikey) {
		return nil, fmt.Errorf("invalid api key")
	}

	levelMap := map[NotifyType]string{}
	for notifyType, level := range pushwardDefaultLevels {
		levelMap[notifyType] = level
		// Each type's level is overridable, as in ?failure=critical.
		if override := resolvePushWardLevel(target.Query[string(notifyType)]); override != "" {
			levelMap[notifyType] = override
		}
	}

	result := &PushWardTarget{
		apikey:   apikey,
		level:    resolvePushWardLevel(target.Query["level"]),
		levelMap: levelMap,
	}

	if raw := strings.TrimSpace(target.Query["volume"]); raw != "" {
		if volume, err := strconv.ParseFloat(raw, 64); err == nil {
			result.volume = &volume
		}
	}

	return result, nil
}

func (p *PushWardTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	level := p.level
	if level == "" {
		level = p.levelMap[notifyType]
	}

	payload := map[string]any{
		"title": title,
		"body":  body,
		"level": level,
	}
	// Upstream always attaches the themed icon; there is no image toggle.
	if icon := appriseImageURL(notifyType, "128x128"); icon != "" {
		payload["icon_url"] = icon
	}
	// Volume only applies to a critical alert.
	if level == "critical" && p.volume != nil {
		payload["volume"] = *p.volume
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    pushwardURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Content-Type":  "application/json; charset=utf-8",
			"Authorization": "Bearer " + p.apikey,
		},
		Body: string(data),
	}, nil
}

func (p *PushWardTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(148, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"apikey": map[string]any{
					"alias_of": "apikey",
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
				"failure": map[string]any{
					"default":  "time-sensitive",
					"map_to":   "failure",
					"name":     "Failure Level",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"passive", "active", "time-sensitive", "critical"},
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
				"info": map[string]any{
					"default":  "active",
					"map_to":   "info",
					"name":     "Info Level",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"passive", "active", "time-sensitive", "critical"},
				},
				"level": map[string]any{
					"map_to":   "level",
					"name":     "Level",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"passive", "active", "time-sensitive", "critical"},
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
				"success": map[string]any{
					"default":  "active",
					"map_to":   "success",
					"name":     "Success Level",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"passive", "active", "time-sensitive", "critical"},
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
				"volume": map[string]any{
					"map_to":   "volume",
					"max":      1.0,
					"min":      0.0,
					"name":     "Volume",
					"private":  false,
					"required": false,
					"type":     "float",
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
				"warning": map[string]any{
					"default":  "time-sensitive",
					"map_to":   "warning",
					"name":     "Warning Level",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"passive", "active", "time-sensitive", "critical"},
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{apikey}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "Integration Key",
					"private":  true,
					"regex":    []string{"^hlk_[A-Za-z0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "pushward",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pushward"},
				},
			},
		},
		"enabled": true,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"protocols":        nil,
		"secure_protocols": []string{"pushward"},
		"service_name":     "PushWard",
		"service_url":      "https://pushward.app/",
		"setup_url":        "https://appriseit.com/services/pushward/",
	})
}
