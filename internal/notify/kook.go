package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	kookAPIURL    = "https://www.kookapp.cn/api/v3"
	kookDMPrefix  = "@"
	kookTypeText  = 1
	kookTypeKMark = 9
	kookModeBot   = "bot"
	kookModeHook  = "webhook"
)

// Channel and user IDs are numeric snowflakes.
var kookTargetPattern = regexp.MustCompile(`^\d{1,20}$`)

var kookModes = []string{kookModeBot, kookModeHook}

type KookTarget struct {
	token    string
	mode     string
	format   string
	channels []string
	dmUsers  []string
}

func NewKookTarget(target *ParsedURL) (*KookTarget, error) {
	// The token is the host, or ?token= which is easier to express in YAML.
	token := strings.TrimSpace(target.Query["token"])
	if token == "" {
		token = strings.TrimSpace(target.Host)
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	mode := kookModeBot
	if raw := strings.TrimSpace(strings.ToLower(target.Query["mode"])); raw != "" {
		// Upstream accepts a prefix, so ?mode=web resolves to webhook.
		matched := ""
		for _, candidate := range kookModes {
			if strings.HasPrefix(candidate, raw) {
				matched = candidate
				break
			}
		}
		if matched == "" {
			return nil, fmt.Errorf("invalid mode: %s", raw)
		}
		mode = matched
	}

	format := normalizeNotifyFormat(target.Query["format"])
	if format == "" {
		// Kook renders KMarkdown natively, so markdown is the default.
		format = "markdown"
	}

	entries := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	kook := &KookTarget{token: token, mode: mode, format: format}
	for _, entry := range entries {
		if user, ok := strings.CutPrefix(entry, kookDMPrefix); ok {
			if kookTargetPattern.MatchString(user) {
				kook.dmUsers = append(kook.dmUsers, user)
			}
			continue
		}

		// A leading # is accepted for convenience and carries no meaning.
		channel := strings.TrimPrefix(entry, "#")
		if kookTargetPattern.MatchString(channel) {
			kook.channels = append(kook.channels, channel)
		}
	}

	if mode == kookModeBot && len(kook.channels) == 0 && len(kook.dmUsers) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return kook, nil
}

func (k *KookTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := k.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (k *KookTarget) Send(body, title string, notifyType NotifyType) error {
	specs, err := k.buildRequests(body, title, notifyType)
	if err != nil {
		return err
	}

	for _, spec := range specs {
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (k *KookTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	// Kook has no title field, so the title is folded into the message.
	// Upstream's 5000 character limit is handled by the framework's overflow
	// handling, which splits rather than truncates and is not ported here.
	content := mergeTitleBody(title, body)

	messageType := kookTypeText
	if k.format == "markdown" {
		messageType = kookTypeKMark
	}

	if k.mode == kookModeHook {
		data, err := json.Marshal(map[string]any{
			"type":    messageType,
			"content": content,
		})
		if err != nil {
			return nil, err
		}

		return []RequestSpec{{
			Method: "POST",
			URL:    fmt.Sprintf("%s/incoming/%s", kookAPIURL, k.token),
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "*/*",
				"Content-Type": "application/json",
			},
			Body: string(data),
		}}, nil
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Authorization": "Bot " + k.token,
		"Content-Type":  "application/json",
	}

	// Channels go to the message endpoint and users to the DM endpoint;
	// upstream sends every channel before the first user.
	type kookEndpoint struct {
		url string
		id  string
	}
	endpoints := make([]kookEndpoint, 0, len(k.channels)+len(k.dmUsers))
	for _, channel := range k.channels {
		endpoints = append(endpoints, kookEndpoint{kookAPIURL + "/message/create", channel})
	}
	for _, user := range k.dmUsers {
		endpoints = append(endpoints, kookEndpoint{kookAPIURL + "/direct-message/create", user})
	}

	specs := make([]RequestSpec, 0, len(endpoints))
	for _, endpoint := range endpoints {
		data, err := json.Marshal(map[string]any{
			"type":      messageType,
			"target_id": endpoint.id,
			"content":   content,
		})
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     endpoint.url,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}
func init() {
	RegisterSchemaEntryOrdered(159, SchemaEntry{
		"attachment_support": true,
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
					"default":  "markdown",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"mode": map[string]any{
					"default":  "bot",
					"map_to":   "mode",
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"bot", "webhook"},
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
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
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
			"templates": []string{"{schema}://{token}", "{schema}://{token}/{targets}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "kook",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"kook"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []any{},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":          true,
		"protocols":        nil,
		"secure_protocols": []string{"kook"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"service_name": "Kook",
		"service_url":  "https://www.kookapp.cn/",
		"setup_url":    "https://appriseit.com/services/kook/",
	})
}
