package notify

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

const fcmLegacyURL = "https://fcm.googleapis.com/fcm/send"

type FCMTarget struct {
	apiKey       string
	project      string
	keyfile      string
	mode         string
	targets      []string
	color        string
	hasColor     bool
	priority     string
	includeImage bool
	imageURL     string
	data         map[string]string
}

func NewFCMTarget(target *ParsedURL) (*FCMTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if apiKey == "" {
		apiKey = strings.TrimSpace(target.User)
	}
	project := strings.TrimSpace(target.Query["project"])
	if project == "" {
		project = strings.TrimSpace(target.Host)
		if project == "" {
			project = strings.TrimSpace(target.User)
		}
	}
	keyfile := strings.TrimSpace(target.Query["keyfile"])
	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode == "" {
		if keyfile != "" {
			mode = "oauth2"
		} else {
			mode = "legacy"
		}
	}
	switch mode {
	case "legacy", "oauth2":
	default:
		return nil, fmt.Errorf("invalid mode")
	}
	if mode == "legacy" && apiKey == "" {
		return nil, fmt.Errorf("missing api key")
	}
	if mode == "oauth2" {
		if project == "" {
			return nil, fmt.Errorf("missing project")
		}
		if keyfile == "" {
			return nil, fmt.Errorf("missing keyfile")
		}
	}

	targets := make([]string, 0, 1)
	for _, entry := range splitPath(target.Path) {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			targets = append(targets, trimmed)
		}
	}
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		targets = append(targets, parseDelimitedList(toValue)...)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	color := ""
	hasColor := false
	if raw, ok := target.Query["color"]; ok {
		color = strings.TrimSpace(raw)
		hasColor = true
	}

	priority := ""
	if raw := strings.TrimSpace(target.Query["priority"]); raw != "" {
		priority = normalizeFCMPriority(raw)
		if priority == "" {
			return nil, fmt.Errorf("invalid priority")
		}
	}

	includeImage := parseBoolWithDefault(target.Query["image"], false)
	imageURL := strings.TrimSpace(target.Query["image_url"])
	if imageURL != "" {
		if _, ok := target.Query["image"]; !ok {
			includeImage = true
		}
	}

	data := map[string]string{}
	for key, value := range target.QueryAdd {
		if strings.TrimSpace(key) == "" {
			continue
		}
		data[key] = value
	}

	return &FCMTarget{
		apiKey:       apiKey,
		project:      project,
		keyfile:      keyfile,
		mode:         mode,
		targets:      targets,
		color:        color,
		hasColor:     hasColor,
		priority:     priority,
		includeImage: includeImage,
		imageURL:     imageURL,
		data:         data,
	}, nil
}

func (f *FCMTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(f.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}
	if f.mode == "oauth2" {
		accessToken, err := f.accessToken()
		if err != nil {
			return RequestSpec{}, err
		}
		return f.buildOAuthSpec(body, title, notifyType, f.targets[0], accessToken)
	}

	spec, err := f.buildSpec(body, title, notifyType, f.targets[0])
	if err != nil {
		return RequestSpec{}, err
	}
	return spec, nil
}

func (f *FCMTarget) Send(body, title string, notifyType NotifyType) error {
	if len(f.targets) == 0 {
		return fmt.Errorf("missing targets")
	}
	if f.mode == "oauth2" {
		accessToken, err := f.accessToken()
		if err != nil {
			return err
		}
		for _, recipient := range f.targets {
			spec, err := f.buildOAuthSpec(body, title, notifyType, recipient, accessToken)
			if err != nil {
				return err
			}
			if err := SendRequest(spec); err != nil {
				return err
			}
		}
		return nil
	}

	for _, recipient := range f.targets {
		spec, err := f.buildSpec(body, title, notifyType, recipient)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (f *FCMTarget) buildSpec(body, title string, notifyType NotifyType, recipient string) (RequestSpec, error) {
	payload := map[string]any{
		"notification": map[string]any{
			"notification": map[string]string{
				"title": title,
				"body":  body,
			},
		},
	}

	if color, ok := f.resolveColor(notifyType); ok {
		payload["notification"].(map[string]any)["notification"].(map[string]string)["color"] = color
	}

	image := ""
	if f.imageURL != "" {
		image = f.imageURL
	} else if f.includeImage {
		image = appriseImageURL(notifyType, "256x256")
	}
	if f.includeImage && image != "" {
		payload["notification"].(map[string]any)["notification"].(map[string]string)["image"] = image
	}

	if len(f.data) > 0 {
		payload["data"] = f.data
	}

	if priority := f.priorityPayload(); priority != "" {
		payload["priority"] = priority
	}

	if strings.HasPrefix(recipient, "#") {
		payload["to"] = "/topics/" + recipient
	} else {
		payload["to"] = recipient
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    fcmLegacyURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  "application/json",
			"Authorization": "key=" + f.apiKey,
		},
		Body: string(data),
	}, nil
}

func (f *FCMTarget) resolveColor(notifyType NotifyType) (string, bool) {
	if !f.hasColor {
		return "", false
	}
	if f.color == "" {
		return appriseColor(notifyType), true
	}

	normalized := strings.ToLower(strings.TrimSpace(f.color))
	switch normalized {
	case "1", "true", "yes", "on", "y":
		return appriseColor(notifyType), true
	case "0", "false", "no", "off", "n":
		return "", false
	default:
		if color, ok := normalizeHexColor(normalized); ok {
			return color, true
		}
	}

	return "", false
}

func normalizeHexColor(raw string) (string, bool) {
	value := strings.TrimPrefix(raw, "#")
	if len(value) != 3 && len(value) != 6 {
		return "", false
	}

	for _, r := range value {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			return "", false
		}
	}

	if len(value) == 3 {
		value = string([]byte{
			value[0], value[0],
			value[1], value[1],
			value[2], value[2],
		})
	}

	return "#" + strings.ToLower(value), true
}

func normalizeFCMPriority(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}
	key := trimmed
	if len(key) > 2 {
		key = key[:2]
	}
	switch {
	case strings.HasPrefix("min", key):
		return "min"
	case strings.HasPrefix("low", key):
		return "low"
	case strings.HasPrefix("normal", key):
		return "normal"
	case strings.HasPrefix("high", key):
		return "high"
	case strings.HasPrefix("max", key):
		return "max"
	default:
		return ""
	}
}

func (f *FCMTarget) priorityPayload() string {
	if f.priority == "" {
		return ""
	}
	switch f.priority {
	case "min", "low", "normal":
		return "normal"
	case "high", "max":
		return "high"
	default:
		return ""
	}
}

func init() {
	RegisterSchemaEntryOrdered(84, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"color": map[string]any{
					"default":  "yes",
					"map_to":   "color",
					"name":     "Notification Color",
					"private":  false,
					"required": false,
					"type":     "string",
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
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"image": map[string]any{
					"default":  false,
					"map_to":   "include_image",
					"name":     "Include Image",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"image_url": map[string]any{
					"map_to":   "image_url",
					"name":     "Custom Image URL",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"mode": map[string]any{
					"default":  "legacy",
					"map_to":   "mode",
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"legacy", "oauth2"},
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
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"min", "low", "normal", "high", "max"},
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
			},
			"kwargs": map[string]any{
				"data_kwargs": map[string]any{
					"map_to":   "data_kwargs",
					"name":     "Data Entries",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{project}/{targets}?keyfile={keyfile}", "{schema}://{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"keyfile": map[string]any{
					"map_to":   "keyfile",
					"name":     "OAuth2 KeyFile",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"project": map[string]any{
					"map_to":   "project",
					"name":     "Project ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "fcm",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"fcm"},
				},
				"target_device": map[string]any{
					"map_to":   "targets",
					"name":     "Target Device",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_topic": map[string]any{
					"map_to":   "targets",
					"name":     "Target Topic",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_device", "target_topic"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []string{"cryptography"},
		},
		"secure_protocols": []string{"fcm"},
		"service_name":     "Firebase Cloud Messaging",
		"service_url":      "https://firebase.google.com",
		"setup_url":        "https://appriseit.com/services/fcm/",
	})
}
