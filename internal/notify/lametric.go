package notify

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const lametricDefaultPort = 8080
const lametricDefaultUser = "dev"
const lametricDefaultPriority = "info"
const lametricDefaultIconType = "none"

var lametricIconMap = map[NotifyType]string{
	NotifyInfo:    "i620",
	NotifySuccess: "i9182",
	NotifyWarning: "i9183",
	NotifyFailure: "i9184",
}

type LametricTarget struct {
	host     string
	port     int
	secure   bool
	user     string
	apiKey   string
	priority string
	iconType string
	icon     string
	cycles   int
}

func NewLametricTarget(target *ParsedURL) (*LametricTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	user := strings.TrimSpace(target.User)
	apiKey := strings.TrimSpace(target.Password)
	if user != "" && apiKey == "" {
		apiKey = user
		user = ""
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(target.Query["apikey"])
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode == "cloud" {
		return nil, fmt.Errorf("cloud mode not supported")
	}

	priority := strings.ToLower(strings.TrimSpace(target.Query["priority"]))
	if priority == "" {
		priority = lametricDefaultPriority
	}
	if !isLametricPriority(priority) {
		priority = lametricDefaultPriority
	}

	iconType := strings.ToLower(strings.TrimSpace(target.Query["icon_type"]))
	if iconType == "" {
		iconType = lametricDefaultIconType
	}
	if !isLametricIconType(iconType) {
		iconType = lametricDefaultIconType
	}

	icon := strings.TrimSpace(target.Query["icon"])
	icon = strings.TrimPrefix(icon, "#")

	cycles := 1
	if raw := strings.TrimSpace(target.Query["cycles"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			cycles = parsed
		}
	}

	port := target.Port
	if port == 0 {
		port = lametricDefaultPort
	}

	return &LametricTarget{
		host:     host,
		port:     port,
		secure:   strings.EqualFold(target.Scheme, "lametrics"),
		user:     user,
		apiKey:   apiKey,
		priority: priority,
		iconType: iconType,
		icon:     icon,
		cycles:   cycles,
	}, nil
}

func (l *LametricTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)

	icon := l.icon
	if icon == "" {
		if mapped, ok := lametricIconMap[notifyType]; ok {
			icon = mapped
		}
	}

	payload := map[string]any{
		"priority":  l.priority,
		"icon_type": l.iconType,
		"lifetime":  120000,
		"model": map[string]any{
			"cycles": l.cycles,
			"frames": []map[string]any{
				{
					"icon": icon,
					"text": message,
				},
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	user := l.user
	if user == "" {
		user = lametricDefaultUser
	}

	return RequestSpec{
		Method: "POST",
		URL:    l.buildURL(),
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Cache-Control": "no-cache",
			"Authorization": basicAuthHeader(user, l.apiKey),
		},
		Body: string(data),
	}, nil
}

func (l *LametricTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := l.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (l *LametricTarget) buildURL() string {
	scheme := "http"
	if l.secure {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d/api/v2/device/notifications", scheme, l.host, l.port)
}

func isLametricPriority(value string) bool {
	switch value {
	case "info", "warning", "critical":
		return true
	default:
		return false
	}
}

func isLametricIconType(value string) bool {
	switch value {
	case "info", "alert", "none":
		return true
	default:
		return false
	}
}

func init() {
	RegisterSchemaEntryOrdered(114, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"apikey": map[string]any{
					"alias_of": "apikey",
				},
				"app_id": map[string]any{
					"alias_of": "app_id",
				},
				"app_token": map[string]any{
					"alias_of": "app_token",
				},
				"app_ver": map[string]any{
					"alias_of": "app_ver",
				},
				"cto": map[string]any{
					"default":  4,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"cycles": map[string]any{
					"default":  1,
					"map_to":   "cycles",
					"min":      0,
					"name":     "Cycles",
					"private":  false,
					"required": false,
					"type":     "int",
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
				"icon": map[string]any{
					"map_to":   "icon",
					"name":     "Custom Icon",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"icon_type": map[string]any{
					"default":  "none",
					"map_to":   "icon_type",
					"name":     "Icon Type",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"info", "alert", "none"},
				},
				"mode": map[string]any{
					"default":  "device",
					"map_to":   "mode",
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"cloud", "device"},
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
					"default":  "info",
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"info", "warning", "critical"},
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
			"templates": []string{"{schema}://{app_token}@{app_id}", "{schema}://{app_token}@{app_id}/{app_ver}", "{schema}://{apikey}@{host}", "{schema}://{user}:{apikey}@{host}", "{schema}://{apikey}@{host}:{port}", "{schema}://{user}:{apikey}@{host}:{port}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "Device API Key",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"app_id": map[string]any{
					"map_to":   "app_id",
					"name":     "App ID",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"app_token": map[string]any{
					"map_to":   "app_token",
					"name":     "App Access Token",
					"private":  false,
					"regex":    []string{"^[A-Z0-9]{80,}==$", "i"},
					"required": false,
					"type":     "string",
				},
				"app_ver": map[string]any{
					"default":  "1",
					"map_to":   "app_ver",
					"name":     "App Version",
					"private":  false,
					"regex":    []string{"^[1-9][0-9]*$", ""},
					"required": false,
					"type":     "string",
				},
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"port": map[string]any{
					"default":  8080,
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
					"values":   []string{"lametric", "lametrics"},
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Username",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"lametric"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"lametrics"},
		"service_name":     "LaMetric",
		"service_url":      "https://lametric.com",
		"setup_url":        "https://appriseit.com/services/lametric/",
	})
}
