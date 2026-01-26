package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const fortySixElksURL = "https://api.46elks.com/a1/sms"

type FortySixElksTarget struct {
	user     string
	password string
	source   string
	targets  []string
}

func NewFortySixElksTarget(target *ParsedURL) (*FortySixElksTarget, error) {
	user := strings.TrimSpace(target.User)
	password := target.Password
	if password == "" {
		return nil, fmt.Errorf("missing password")
	}
	if user == "" {
		return nil, fmt.Errorf("missing user")
	}

	source := ""
	if rawSource, ok := target.Query["from"]; ok && rawSource != "" {
		source = rawSource
	} else if target.Host != "" {
		source = target.Host
	}
	source = strings.TrimSpace(source)

	targets := []string{}
	appendTarget := func(raw string) {
		if normalized, ok := normalizeElksTarget(raw); ok {
			targets = append(targets, normalized)
		}
	}

	for _, entry := range splitPath(target.Path) {
		appendTarget(entry)
	}
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			appendTarget(entry)
		}
	}

	if len(targets) == 0 {
		if normalized, ok := normalizeElksTarget(source); ok {
			targets = append(targets, normalized)
		}
	}

	return &FortySixElksTarget{
		user:     user,
		password: password,
		source:   source,
		targets:  targets,
	}, nil
}

func (f *FortySixElksTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(f.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	spec, err := f.buildRequest(f.targets[0], message)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (f *FortySixElksTarget) Send(body, title string, notifyType NotifyType) error {
	if len(f.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	for _, target := range f.targets {
		spec, err := f.buildRequest(target, message)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (f *FortySixElksTarget) buildRequest(target, message string) (RequestSpec, error) {
	payload := url.Values{}
	payload.Set("to", target)
	payload.Set("message", message)
	if f.source != "" {
		payload.Set("from", f.source)
	}

	return RequestSpec{
		Method: "POST",
		URL:    fortySixElksURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Authorization": basicAuthHeader(f.user, f.password),
			"Content-Type":  "application/x-www-form-urlencoded",
		},
		Body: payload.Encode(),
	}, nil
}

func normalizeElksTarget(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	hasPlus := strings.HasPrefix(trimmed, "+")
	normalized, ok := normalizePhone(trimmed)
	if !ok {
		return "", false
	}
	if hasPlus {
		return "+" + normalized, true
	}
	return normalized, true
}

func init() {
	RegisterSchemaEntryOrdered(44, SchemaEntry{
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
				"from": map[string]any{
					"alias_of": "from_phone",
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{user}:{password}@/{from_phone}", "{schema}://{user}:{password}@/{from_phone}/{targets}"},
			"tokens": map[string]any{
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "password",
					"name":     "API Password",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"46elks", "elks"},
				},
				"target_phone": map[string]any{
					"map_to":   "targets",
					"name":     "Target Phone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "API Username",
					"private":  false,
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
		"secure_protocols": []string{"46elks", "elks"},
		"service_name":     "46elks",
		"service_url":      "https://46elks.com",
		"setup_url":        "https://appriseit.com/services/46elks/",
	})
}
