package notify

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var joinGroupRe = regexp.MustCompile(`^(group\.)?(all|android|chrome|windows10|phone|tablet|pc)$`)
var joinDeviceRe = regexp.MustCompile(`^[a-z0-9]{32}$`)

type JoinTarget struct {
	apiKey       string
	targets      []string
	includeImage bool
	priority     int
}

func NewJoinTarget(target *ParsedURL) (*JoinTarget, error) {
	apiKey := strings.TrimSpace(target.User)
	if apiKey == "" {
		apiKey = strings.TrimSpace(target.Host)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	targets := []string{}
	if target.User != "" && target.Host != "" {
		targets = append(targets, target.Host)
	}
	targets = append(targets, splitPath(target.Path)...)
	if toValue, ok := target.Query["to"]; ok && strings.TrimSpace(toValue) != "" {
		targets = append(targets, parseDelimitedList(toValue)...)
	}
	if len(targets) == 0 {
		targets = append(targets, "group.all")
	}

	priority := 0
	if rawPriority := strings.TrimSpace(target.Query["priority"]); rawPriority != "" {
		switch strings.ToLower(rawPriority) {
		case "low", "l", "-2":
			priority = -2
		case "moderate", "m", "-1":
			priority = -1
		case "normal", "n", "0":
			priority = 0
		case "high", "h", "1":
			priority = 1
		case "emergency", "e", "2":
			priority = 2
		}
	}

	return &JoinTarget{
		apiKey:       apiKey,
		targets:      targets,
		includeImage: parseBool(target.Query["image"], true),
		priority:     priority,
	}, nil
}

func (j *JoinTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := j.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (j *JoinTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(j.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	target := j.targets[0]
	args := url.Values{}
	args.Set("apikey", j.apiKey)
	args.Set("priority", fmt.Sprintf("%d", j.priority))
	args.Set("title", title)
	args.Set("text", body)

	if joinDeviceRe.MatchString(target) || joinGroupRe.MatchString(target) {
		if joinGroupRe.MatchString(target) && !strings.HasPrefix(target, "group.") {
			target = "group." + target
		}
		args.Set("deviceId", target)
	} else {
		args.Set("deviceNames", target)
	}

	if j.includeImage {
		args.Set("icon", appriseImageURL(notifyType, "72x72"))
	}

	u := url.URL{
		Scheme:   "https",
		Host:     "joinjoaomgcd.appspot.com",
		Path:     "/_ah/api/messaging/v1/sendPush",
		RawQuery: args.Encode(),
	}

	return RequestSpec{
		Method: "POST",
		URL:    u.String(),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: "",
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(51, SchemaEntry{
		"attachment_support": false,
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
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[a-z0-9]{32}$", "i"},
					"required": true,
					"type":     "string",
				},
				"device": map[string]any{
					"map_to":   "targets",
					"name":     "Device ID",
					"private":  false,
					"regex":    []string{"^[a-z0-9]{32}$", "i"},
					"required": false,
					"type":     "string",
				},
				"device_name": map[string]any{
					"map_to":   "targets",
					"name":     "Device Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"group": map[string]any{
					"map_to":   "targets",
					"name":     "Group",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"all", "android", "chrome", "windows10", "phone", "tablet", "pc"},
				},
				"schema": map[string]any{
					"default":  "join",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"join"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"device", "device_name", "group"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
		"secure_protocols": []string{"join"},
		"service_name":     "Join",
		"service_url":      "https://joaoapps.com/join/",
		"setup_url":        "https://appriseit.com/services/join/",
	})
}
