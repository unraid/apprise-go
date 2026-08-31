package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	pingletPrioritySilent = "silent"
	pingletPriorityNormal = "normal"
	pingletPriorityUrgent = "urgent"
)

const (
	pingletMaxBadgeCount    = 3
	pingletMaxBadgeKeyLen   = 24
	pingletMaxBadgeValueLen = 32
	pingletMaxDataKeyLen    = 64
	pingletMaxDataValueLen  = 256
)

// pingletLevel maps the Apprise notification type to Pinglet's display-only
// level.
func pingletLevel(notifyType NotifyType) string {
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

type PingletTarget struct {
	token     string
	host      string
	port      int
	hasPort   bool
	secure    bool
	fullpath  string
	namespace string
	topic     string
	priority  string
	badges    map[string]string
	data      map[string]string
}

func NewPingletTarget(target *ParsedURL) (*PingletTarget, error) {
	secure := strings.EqualFold(target.Scheme, "pinglets")

	token := strings.TrimSpace(target.User)
	if value := strings.TrimSpace(target.Query["token"]); value != "" {
		token = value
	}
	if token == "" {
		return nil, fmt.Errorf("missing api key")
	}

	if strings.TrimSpace(target.Host) == "" {
		return nil, fmt.Errorf("missing host")
	}

	entries := splitPath(target.Path)
	if len(entries) < 2 {
		return nil, fmt.Errorf("missing namespace/topic")
	}
	topic := entries[len(entries)-1]
	namespace := entries[len(entries)-2]
	entries = entries[:len(entries)-2]

	fullpath := "/"
	if len(entries) > 0 {
		fullpath = "/" + strings.Join(entries, "/") + "/"
	}

	priority := pingletPriorityNormal
	if raw := strings.ToLower(strings.TrimSpace(target.Query["priority"])); raw != "" {
		switch {
		case strings.HasPrefix(raw, "s"):
			priority = pingletPrioritySilent
		case strings.HasPrefix(raw, "n"):
			priority = pingletPriorityNormal
		case strings.HasPrefix(raw, "u"):
			priority = pingletPriorityUrgent
		}
	}

	// Badges (pills rendered on the feed card); the server rejects
	// over-length keys/values outright, so they are truncated instead.
	badges := map[string]string{}
	for _, key := range target.QueryPayloadOrder {
		if len(badges) >= pingletMaxBadgeCount {
			break
		}
		value := target.QueryPayload[key]
		badges[truncateString(key, pingletMaxBadgeKeyLen)] = truncateString(value, pingletMaxBadgeValueLen)
	}

	// Metadata key/value pairs (shown on the detail sheet).
	data := map[string]string{}
	for key, value := range target.QueryAdd {
		data[truncateString(key, pingletMaxDataKeyLen)] = truncateString(value, pingletMaxDataValueLen)
	}

	return &PingletTarget{
		token:     token,
		host:      target.Host,
		port:      target.Port,
		hasPort:   target.HasPort,
		secure:    secure,
		fullpath:  fullpath,
		namespace: namespace,
		topic:     topic,
		priority:  priority,
		badges:    badges,
		data:      data,
	}, nil
}

func truncateString(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func (p *PingletTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}
	return SendRequest(spec)
}

func (p *PingletTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	scheme := "http"
	if p.secure {
		scheme = "https"
	}
	url := scheme + "://" + p.host
	if p.hasPort {
		url += fmt.Sprintf(":%d", p.port)
	}
	url += p.fullpath + p.namespace + "/" + p.topic

	payload := map[string]any{
		"message":  body,
		"priority": p.priority,
		"level":    pingletLevel(notifyType),
	}
	if title != "" {
		payload["title"] = title
	}
	if len(p.badges) > 0 {
		payload["badges"] = p.badges
	}
	if len(p.data) > 0 {
		payload["data"] = p.data
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + p.token,
		},
		Body: string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(167, SchemaEntry{
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
				"format": map[string]any{
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
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
					"default":  "normal",
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"silent", "normal", "urgent"},
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
			"kwargs": map[string]any{
				"badges": map[string]any{
					"map_to":   "badges",
					"name":     "Badges",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"data": map[string]any{
					"map_to":   "data",
					"name":     "Metadata",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{token}@{host}/{namespace}/{topic}", "{schema}://{token}@{host}:{port}/{namespace}/{topic}", "{schema}://{token}@{host}{path}{namespace}/{topic}", "{schema}://{token}@{host}:{port}{path}{namespace}/{topic}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"namespace": map[string]any{
					"map_to":   "namespace",
					"name":     "Namespace",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"path": map[string]any{
					"default":  "/",
					"map_to":   "fullpath",
					"name":     "Path",
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
					"values":   []string{"pinglet", "pinglets"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"topic": map[string]any{
					"map_to":   "topic",
					"name":     "Topic",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"pinglet"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"pinglets"},
		"service_name":     "Pinglet",
		"service_url":      "https://pinglet.co.uk/",
		"setup_url":        "https://appriseit.com/services/pinglet/",
	})
}
