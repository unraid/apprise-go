package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const msg91URL = "https://control.msg91.com/api/v5/flow/"

var msg91ReservedKeys = map[string]struct{}{
	"mobiles": {},
}

type MSG91Target struct {
	template        string
	authKey         string
	shortURL        bool
	templateMapping map[string]string
	targets         []string
}

func NewMSG91Target(target *ParsedURL) (*MSG91Target, error) {
	template := strings.TrimSpace(target.User)
	authKey := strings.TrimSpace(target.Host)
	if template == "" || authKey == "" {
		return nil, fmt.Errorf("missing template or authkey")
	}

	shortURL := false
	if raw := target.Query["short_url"]; raw != "" {
		shortURL = parseBool(raw, false)
	}

	targets := []string{}
	appendTarget := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if normalized, ok := normalizePhone(raw); ok {
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

	mapping := map[string]string{}
	for key, value := range target.QueryPayload {
		mapping[key] = value
	}

	return &MSG91Target{
		template:        template,
		authKey:         authKey,
		shortURL:        shortURL,
		templateMapping: mapping,
		targets:         targets,
	}, nil
}

func (m *MSG91Target) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(m.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	payload, err := m.buildPayload(message, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    msg91URL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
			"authkey":      m.authKey,
		},
		Body: string(data),
	}, nil
}

func (m *MSG91Target) Send(body, title string, notifyType NotifyType) error {
	if len(m.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	payload, err := m.buildPayload(message, notifyType)
	if err != nil {
		return err
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	spec := RequestSpec{
		Method: "POST",
		URL:    msg91URL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
			"authkey":      m.authKey,
		},
		Body: string(data),
	}

	return SendRequest(spec)
}

func (m *MSG91Target) buildPayload(message string, notifyType NotifyType) (map[string]any, error) {
	recipientPayload := map[string]any{
		"mobiles": "",
		"body":    message,
		"type":    string(notifyType),
	}

	for key, value := range m.templateMapping {
		if _, reserved := msg91ReservedKeys[key]; reserved {
			continue
		}

		if existing, ok := recipientPayload[key]; ok {
			if strings.TrimSpace(value) == "" {
				delete(recipientPayload, key)
				continue
			}
			recipientPayload[value] = existing
			delete(recipientPayload, key)
			continue
		}

		recipientPayload[key] = value
	}

	recipients := make([]map[string]any, 0, len(m.targets))
	for _, target := range m.targets {
		recipient := cloneAnyMap(recipientPayload)
		recipient["mobiles"] = target
		recipients = append(recipients, recipient)
	}

	shortURL := 0
	if m.shortURL {
		shortURL = 1
	}

	return map[string]any{
		"template_id": m.template,
		"short_url":   shortURL,
		"recipients":  recipients,
	}, nil
}

func cloneAnyMap(input map[string]any) map[string]any {
	clone := make(map[string]any, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}

func init() {
	RegisterSchemaEntryOrdered(75, SchemaEntry{
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
				"short_url": map[string]any{
					"default":  false,
					"map_to":   "short_url",
					"name":     "Short URL",
					"private":  false,
					"required": false,
					"type":     "bool",
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
				"template_mapping": map[string]any{
					"map_to":   "template_mapping",
					"name":     "Template Mapping",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{template}@{authkey}/{targets}"},
			"tokens": map[string]any{
				"authkey": map[string]any{
					"map_to":   "authkey",
					"name":     "Authentication Key",
					"private":  true,
					"regex":    []string{"^[a-z0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "msg91",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"msg91"},
				},
				"target_phone": map[string]any{
					"map_to":   "targets",
					"name":     "Target Phone No",
					"prefix":   "+",
					"private":  false,
					"regex":    []string{"^[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"template": map[string]any{
					"map_to":   "template",
					"name":     "Template ID",
					"private":  true,
					"regex":    []string{"^[a-z0-9 _-]+$", "i"},
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
		"secure_protocols": []string{"msg91"},
		"service_name":     "MSG91",
		"service_url":      "https://msg91.com",
		"setup_url":        "https://appriseit.com/services/msg91/",
	})
}
