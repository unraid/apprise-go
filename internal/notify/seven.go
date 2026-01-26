package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const sevenURL = "https://gateway.seven.io/api/sms"

type SevenTarget struct {
	apiKey  string
	source  string
	flash   bool
	label   string
	targets []string
}

func NewSevenTarget(target *ParsedURL) (*SevenTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	source := ""
	if rawSource, ok := target.Query["from"]; ok && rawSource != "" {
		source = rawSource
	} else if rawSource, ok := target.Query["source"]; ok && rawSource != "" {
		source = rawSource
	}
	source = strings.TrimSpace(source)

	flash := parseBool(target.Query["flash"], false)
	label := strings.TrimSpace(target.Query["label"])

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

	return &SevenTarget{
		apiKey:  apiKey,
		source:  source,
		flash:   flash,
		label:   label,
		targets: targets,
	}, nil
}

func (s *SevenTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(s.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	spec, err := s.buildRequest(s.targets[0], message)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (s *SevenTarget) Send(body, title string, notifyType NotifyType) error {
	if len(s.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	for _, target := range s.targets {
		spec, err := s.buildRequest(target, message)
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

func (s *SevenTarget) buildRequest(target, message string) (RequestSpec, error) {
	payload := map[string]any{
		"to":   "+" + target,
		"text": message,
	}
	if s.source != "" {
		payload["from"] = s.source
	}
	if s.flash {
		payload["flash"] = s.flash
	}
	if s.label != "" {
		payload["label"] = s.label
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    sevenURL,
		Headers: map[string]string{
			"Accept":       "application/json",
			"Content-Type": "application/json",
			"SentWith":     "Apprise",
			"X-Api-Key":    s.apiKey,
		},
		Body: string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(95, SchemaEntry{
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
				"flash": map[string]any{
					"default":  false,
					"map_to":   "flash",
					"name":     "Flash",
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
				"label": map[string]any{
					"map_to":   "label",
					"name":     "Label",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"source": map[string]any{
					"map_to":   "source",
					"name":     "Originating Address",
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
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "seven",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"seven"},
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
		"secure_protocols": []string{"seven"},
		"service_name":     "seven",
		"service_url":      "https://www.seven.io",
		"setup_url":        "https://appriseit.com/services/seven/",
	})
}
