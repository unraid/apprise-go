package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

type RocketChatTarget struct {
	webhook string
	host    string
	port    int
	secure  bool
	avatar  bool
	targets []string
}

func NewRocketChatTarget(target *ParsedURL) (*RocketChatTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode != "" && mode != "webhook" {
		return nil, fmt.Errorf("unsupported mode: %s", mode)
	}

	webhook := strings.TrimSpace(target.User)
	if target.Password != "" {
		webhook = strings.TrimSpace(target.Password)
	}
	if override, ok := target.Query["webhook"]; ok && strings.TrimSpace(override) != "" {
		webhook = strings.TrimSpace(override)
	}
	if webhook == "" {
		return nil, fmt.Errorf("missing webhook")
	}

	avatar := parseBool(target.Query["avatar"], true)

	targets := splitPath(target.Path)
	if toValue, ok := target.Query["to"]; ok && strings.TrimSpace(toValue) != "" {
		targets = append(targets, parseDelimitedList(toValue)...)
	}

	return &RocketChatTarget{
		webhook: webhook,
		host:    host,
		port:    target.Port,
		secure:  target.Scheme == "rockets",
		avatar:  avatar,
		targets: targets,
	}, nil
}

func (r *RocketChatTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := r.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (r *RocketChatTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"text": mergeTitleBody(title, body),
	}
	if r.avatar {
		payload["avatar"] = appriseImageURL(notifyType, "128x128")
	}
	if len(r.targets) > 0 {
		payload["channel"] = r.targets[0]
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	scheme := "http"
	if r.secure {
		scheme = "https"
	}
	host := r.host
	if r.port != 0 {
		host = fmt.Sprintf("%s:%d", host, r.port)
	}
	url := fmt.Sprintf("%s://%s/hooks/%s", scheme, host, r.webhook)

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}
	return RequestSpec{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(120, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"avatar": map[string]any{
					"default":  false,
					"map_to":   "avatar",
					"name":     "Use Avatar",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
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
					"default":  "markdown",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"mode": map[string]any{
					"map_to":   "mode",
					"name":     "Webhook Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"webhook", "token", "basic"},
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
				"webhook": map[string]any{
					"alias_of": "webhook",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{user}:{password}@{host}:{port}/{targets}", "{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{token}@{host}:{port}/{targets}", "{schema}://{user}:{token}@{host}/{targets}", "{schema}://{webhook}@{host}", "{schema}://{webhook}@{host}:{port}", "{schema}://{webhook}@{host}/{targets}", "{schema}://{webhook}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
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
					"values":   []string{"rocket", "rockets"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_room": map[string]any{
					"map_to":   "targets",
					"name":     "Target Room ID",
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
					"group":    []string{"target_channel", "target_room", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "password",
					"name":     "API Token",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Username",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"webhook": map[string]any{
					"map_to":   "webhook",
					"name":     "Webhook",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"rocket"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"rockets"},
		"service_name":     "Rocket.Chat",
		"service_url":      "https://rocket.chat/",
		"setup_url":        "https://appriseit.com/services/rocketchat/",
	})
}
