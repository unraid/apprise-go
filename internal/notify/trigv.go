package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	trigvDefaultChannel   = "general"
	trigvDefaultNotifyURL = "https://api.trigv.com/api/v1/events"
	trigvTitleMax         = 255
	trigvBodyMax          = 1000
)

var (
	trigvAPIKeyRe  = regexp.MustCompile(`(?i)^trgv_[a-zA-Z0-9]{8}_[a-zA-Z0-9]{32}$`)
	trigvChannelRe = regexp.MustCompile(`(?i)^[a-z0-9][a-z0-9_-]{0,119}$`)
)

func trigvLevel(notifyType NotifyType) string {
	switch notifyType {
	case NotifyInfo:
		return "info"
	case NotifySuccess:
		return "success"
	case NotifyWarning:
		return "warning"
	case NotifyFailure:
		return "error"
	default:
		return "info"
	}
}

type TrigvTarget struct {
	apiKey          string
	targets         []string
	supplementalURL string
	imageURL        string
	eventType       string
	urgency         string
	priority        *int
	notifyURL       string
}

func NewTrigvTarget(target *ParsedURL) (*TrigvTarget, error) {
	secure := strings.EqualFold(target.Scheme, "trigvs")

	// The API key is either the user portion of a user@host URL, or the
	// host itself when no custom hostname is in play.
	apiKey := strings.TrimSpace(target.User)
	host := target.Host
	if apiKey == "" {
		apiKey = strings.TrimSpace(target.Host)
		host = ""
	}
	if !trigvAPIKeyRe.MatchString(apiKey) {
		return nil, fmt.Errorf("invalid api key: %s", apiKey)
	}

	entries := splitPath(target.Path)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	// Upstream runs targets through parse_list, which dedupes and sorts.
	targets := []string{}
	seen := map[string]struct{}{}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !trigvChannelRe.MatchString(entry) {
			return nil, fmt.Errorf("invalid channel: %s", entry)
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		targets = append(targets, entry)
	}
	sort.Strings(targets)
	if len(targets) == 0 {
		targets = append(targets, trigvDefaultChannel)
	}

	urgency := "standard"
	if raw := strings.ToLower(strings.TrimSpace(target.Query["urgency"])); raw != "" {
		urgency = raw
	}
	if urgency != "standard" && urgency != "time_sensitive" {
		return nil, fmt.Errorf("invalid urgency: %s", urgency)
	}

	// Pushover-style priority; ignore anything that does not parse as an
	// int rather than rejecting the whole notification.
	var priority *int
	if raw := strings.TrimSpace(target.Query["priority"]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			priority = &value
		}
	}

	notifyURL := trigvDefaultNotifyURL
	if host != "" {
		scheme := "http"
		if secure {
			scheme = "https"
		}
		port := ""
		if target.HasPort {
			port = fmt.Sprintf(":%d", target.Port)
		}
		notifyURL = fmt.Sprintf("%s://%s%s/api/v1/events", scheme, host, port)
	}

	return &TrigvTarget{
		apiKey:          apiKey,
		targets:         targets,
		supplementalURL: strings.TrimSpace(target.Query["url"]),
		imageURL:        strings.TrimSpace(target.Query["image_url"]),
		eventType:       strings.TrimSpace(target.Query["event_type"]),
		urgency:         urgency,
		priority:        priority,
		notifyURL:       notifyURL,
	}, nil
}

// resolveUrgency maps explicit urgency, Pushover-style priority, or defaults.
func (t *TrigvTarget) resolveUrgency(notifyType NotifyType) string {
	if t.urgency != "standard" {
		return t.urgency
	}
	if t.priority != nil && *t.priority >= 1 {
		return "time_sensitive"
	}
	if notifyType == NotifyFailure {
		return "time_sensitive"
	}
	return "standard"
}

func (t *TrigvTarget) buildRequestFor(body, title string, notifyType NotifyType, channel string) (RequestSpec, error) {
	// Trigv requires a title; fall back to our app description.
	resolvedTitle := title
	if resolvedTitle == "" {
		resolvedTitle = appriseAppDesc
	}
	if body == "" && title == "" {
		body = resolvedTitle
	}

	payload := map[string]any{
		"channel": channel,
		"title":   truncateString(resolvedTitle, trigvTitleMax),
		"level":   trigvLevel(notifyType),
	}
	if body != "" {
		payload["description"] = truncateString(body, trigvBodyMax)
	}
	if t.supplementalURL != "" {
		payload["url"] = t.supplementalURL
	}
	if t.imageURL != "" {
		payload["image_url"] = t.imageURL
	}
	if t.eventType != "" {
		payload["event_type"] = t.eventType
	}
	// Trigv's own API field is delivery_urgency; the shorter urgency= is
	// only the Apprise-facing parameter name.
	payload["delivery_urgency"] = t.resolveUrgency(notifyType)

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    t.notifyURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer " + t.apiKey,
		},
		Body: string(data),
	}, nil
}

func (t *TrigvTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return t.buildRequestFor(body, title, notifyType, t.targets[0])
}

func (t *TrigvTarget) Send(body, title string, notifyType NotifyType) error {
	// Upstream keeps going after a failed channel; see sendOutcome.
	var outcome sendOutcome
	for _, channel := range t.targets {
		spec, err := t.buildRequestFor(body, title, notifyType, channel)
		if err != nil {
			return err
		}
		outcome.record(SendRequest(spec))
	}
	return outcome.err()
}

func init() {
	RegisterSchemaEntryOrdered(168, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
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
				"event_type": map[string]any{
					"map_to":   "event_type",
					"name":     "Event type",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"image_url": map[string]any{
					"map_to":   "image_url",
					"name":     "Image URL",
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
					"map_to":   "priority",
					"name":     "Priority (Pushover compatibility)",
					"private":  false,
					"required": false,
					"type":     "int",
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
				"urgency": map[string]any{
					"default":  "standard",
					"map_to":   "urgency",
					"name":     "Urgency",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"standard", "time_sensitive"},
				},
				"url": map[string]any{
					"map_to":   "supplemental_url",
					"name":     "URL",
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
			"templates": []string{"{schema}://{api_key}", "{schema}://{api_key}/{targets}", "{schema}://{api_key}@{host}/{targets}", "{schema}://{api_key}@{host}:{port}/{targets}", "{schema}://{api_key}@{host}"},
			"tokens": map[string]any{
				"api_key": map[string]any{
					"map_to":   "api_key",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^trgv_[a-zA-Z0-9]{8}_[a-zA-Z0-9]{32}$", "i"},
					"required": true,
					"type":     "string",
				},
				"host": map[string]any{
					"map_to":   "host",
					"name":     "API Hostname",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"port": map[string]any{
					"map_to":   "port",
					"max":      65535,
					"min":      1,
					"name":     "Port",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"trigv", "trigvs"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel",
					"private":  false,
					"required": false,
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
		"protocols": []string{"trigv"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"trigvs"},
		"service_name":     "Trigv",
		"service_url":      "https://trigv.com/",
		"setup_url":        "https://trigv.com/docs/learn/api-keys",
	})
}
