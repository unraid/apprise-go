package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const messageBirdURL = "https://rest.messagebird.com/messages"

type MessageBirdTarget struct {
	apiKey  string
	source  string
	targets []string
}

func NewMessageBirdTarget(target *ParsedURL) (*MessageBirdTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	entries := splitPath(target.Path)
	sourceRaw := ""
	if len(entries) > 0 {
		sourceRaw = entries[0]
		entries = entries[1:]
	}
	if rawSource, ok := target.Query["from"]; ok && rawSource != "" {
		sourceRaw = rawSource
	}
	sourceRaw = strings.TrimSpace(sourceRaw)

	source, ok := normalizePhone(sourceRaw)
	if !ok {
		return nil, fmt.Errorf("invalid source")
	}

	targets := []string{}
	hasTargetInput := false
	appendTarget := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		hasTargetInput = true
		if normalized, ok := normalizePhone(raw); ok {
			targets = append(targets, normalized)
		}
	}

	for _, entry := range entries {
		appendTarget(entry)
	}
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			appendTarget(entry)
		}
	}

	if len(targets) == 0 && !hasTargetInput {
		targets = append(targets, source)
	}

	return &MessageBirdTarget{
		apiKey:  apiKey,
		source:  source,
		targets: targets,
	}, nil
}

func (m *MessageBirdTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(m.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	spec, err := m.buildRequest(m.targets[0], message)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (m *MessageBirdTarget) Send(body, title string, notifyType NotifyType) error {
	if len(m.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	for _, target := range m.targets {
		spec, err := m.buildRequest(target, message)
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

func (m *MessageBirdTarget) buildRequest(target, message string) (RequestSpec, error) {
	payload := url.Values{}
	payload.Set("originator", "+"+m.source)
	payload.Set("recipients", "+"+target)
	payload.Set("body", message)

	return RequestSpec{
		Method: "POST",
		URL:    messageBirdURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Content-Type":  "application/x-www-form-urlencoded",
			"Authorization": fmt.Sprintf("AccessKey %s", m.apiKey),
		},
		Body: payload.Encode(),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(11, SchemaEntry{
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
					"alias_of": "source",
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
			"templates": []string{"{schema}://{apikey}/{source}", "{schema}://{apikey}/{source}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[a-z0-9]{25}$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "msgbird",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"msgbird"},
				},
				"source": map[string]any{
					"map_to":   "source",
					"name":     "Source Phone No",
					"prefix":   "+",
					"private":  false,
					"regex":    []string{"^[0-9\\s)(+-]+$", "i"},
					"required": true,
					"type":     "string",
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
		"secure_protocols": []string{"msgbird"},
		"service_name":     "MessageBird",
		"service_url":      "https://messagebird.com",
		"setup_url":        "https://appriseit.com/services/messagebird/",
	})
}
