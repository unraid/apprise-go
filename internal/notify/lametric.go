package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const lametricDefaultPort = 8080
const lametricDefaultUser = "dev"
const lametricDefaultPriority = "info"
const lametricDefaultIconType = "none"
const lametricDefaultAppVer = "1"

var lametricIconMap = map[NotifyType]string{
	NotifyInfo:    "i620",
	NotifySuccess: "i9182",
	NotifyWarning: "i9183",
	NotifyFailure: "i9184",
}

var lametricAppTokenRe = regexp.MustCompile(`(?i)^[A-Z0-9]{80,}==$`)
var lametricAppIDRe = regexp.MustCompile(`(?i)^(?:com\\.lametric\\.)?([0-9a-z.-]{1,64})(?:/([1-9][0-9]*))?$`)

type LametricTarget struct {
	mode     string
	host     string
	port     int
	secure   bool
	user     string
	apiKey   string
	appID    string
	appVer   string
	appToken string
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
	password := strings.TrimSpace(target.Password)
	if user != "" && password == "" {
		password = user
		user = ""
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode != "cloud" && mode != "device" {
		mode = ""
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

	appID := strings.TrimSpace(target.Query["app_id"])
	if appID == "" {
		appID = strings.TrimSpace(target.Query["app"])
	}
	appVer := strings.TrimSpace(target.Query["app_ver"])
	appToken := strings.TrimSpace(target.Query["app_token"])
	if appToken == "" {
		appToken = strings.TrimSpace(target.Query["token"])
	}

	if appVer == "" {
		if segments := splitPath(target.Path); len(segments) > 0 {
			appVer = strings.TrimSpace(segments[0])
		}
	}

	if appID == "" {
		appID = host
	}

	if mode == "" {
		switch {
		case appToken != "":
			mode = "cloud"
		case appID != "" && appID != host:
			mode = "cloud"
		case password != "" && lametricAppTokenRe.MatchString(password):
			mode = "cloud"
		default:
			mode = "device"
		}
	}

	if mode == "cloud" {
		if appToken == "" {
			appToken = password
		}
		if !lametricAppTokenRe.MatchString(appToken) {
			return nil, fmt.Errorf("invalid app token")
		}

		appIDParsed, appVerParsed, ok := parseLametricAppID(appID)
		if !ok {
			return nil, fmt.Errorf("invalid app id")
		}
		if appVer == "" {
			appVer = appVerParsed
		}
		if appVer == "" {
			appVer = lametricDefaultAppVer
		}
		if !isLametricAppVer(appVer) {
			return nil, fmt.Errorf("invalid app version")
		}

		return &LametricTarget{
			mode:     "cloud",
			secure:   true,
			appID:    appIDParsed,
			appVer:   appVer,
			appToken: appToken,
			priority: priority,
			iconType: iconType,
			icon:     icon,
			cycles:   cycles,
		}, nil
	}

	apiKey := strings.TrimSpace(password)
	if apiKey == "" {
		apiKey = strings.TrimSpace(target.Query["apikey"])
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	return &LametricTarget{
		mode:     "device",
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

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Cache-Control": "no-cache",
	}

	requestURL := ""
	payload := map[string]any{}
	if l.mode == "cloud" {
		requestURL = fmt.Sprintf(
			"https://developer.lametric.com/api/v1/dev/widget/update/com.lametric.%s/%s",
			l.appID,
			l.appVer,
		)
		if l.apiKey != "" {
			headers["X-Access-Token"] = l.apiKey
		}
		payload = map[string]any{
			"frames": []map[string]any{
				{
					"icon":  icon,
					"text":  message,
					"index": 0,
				},
			},
		}
	} else {
		requestURL = l.buildURL()
		payload = map[string]any{
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
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	if l.mode != "cloud" {
		user := l.user
		if user == "" {
			user = lametricDefaultUser
		}
		headers["Authorization"] = basicAuthHeader(user, l.apiKey)
	}

	return RequestSpec{
		Method:  "POST",
		URL:     requestURL,
		Headers: headers,
		Body:    string(data),
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

func isLametricAppVer(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return value[0] != '0'
}

func parseLametricAppID(value string) (string, string, bool) {
	matches := lametricAppIDRe.FindStringSubmatch(strings.TrimSpace(value))
	if matches == nil {
		return "", "", false
	}
	appID := matches[1]
	if strings.HasPrefix(strings.ToLower(appID), "com.lametric.") {
		appID = appID[len("com.lametric."):]
	}
	appVer := ""
	if len(matches) > 2 {
		appVer = matches[2]
	}
	return appID, appVer, true
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
