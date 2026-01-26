package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const httpSMSURL = "https://api.httpsms.com/v1/messages/send"

type HttpSMSTarget struct {
	apiKey  string
	source  string
	targets []string
}

func NewHttpSMSTarget(target *ParsedURL) (*HttpSMSTarget, error) {
	apiKey := target.User
	if rawKey, ok := target.Query["key"]; ok && rawKey != "" {
		apiKey = rawKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	sourceRaw := ""
	targets := []string{}
	hasInvalidTarget := false

	appendTarget := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if normalized, ok := normalizePhone(raw); ok {
			targets = append(targets, normalized)
			return
		}
		hasInvalidTarget = true
	}

	if fromValue, ok := target.Query["from"]; ok && fromValue != "" {
		sourceRaw = fromValue
		appendTarget(target.Host)
		for _, entry := range splitPath(target.Path) {
			appendTarget(entry)
		}
	} else {
		sourceRaw = target.Host
		for _, entry := range splitPath(target.Path) {
			appendTarget(entry)
		}
	}

	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			appendTarget(entry)
		}
	}

	source, ok := normalizePhone(sourceRaw)
	if !ok {
		return nil, fmt.Errorf("invalid source")
	}

	if len(targets) == 0 && !hasInvalidTarget {
		targets = append(targets, source)
	}

	return &HttpSMSTarget{
		apiKey:  apiKey,
		source:  source,
		targets: targets,
	}, nil
}

func (h *HttpSMSTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(h.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	payload := map[string]string{
		"from":    "+" + h.source,
		"to":      "+" + h.targets[0],
		"content": message,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    httpSMSURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
			"x-api-key":    h.apiKey,
		},
		Body: string(data),
	}, nil
}

func (h *HttpSMSTarget) Send(body, title string, notifyType NotifyType) error {
	if len(h.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	for _, target := range h.targets {
		payload := map[string]string{
			"from":    "+" + h.source,
			"to":      "+" + target,
			"content": message,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    httpSMSURL,
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "*/*",
				"Content-Type": "application/json",
				"x-api-key":    h.apiKey,
			},
			Body: string(data),
		}

		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	RegisterSchemaEntryOrdered(7, SchemaEntry{
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
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"key": map[string]any{
					"alias_of": "apikey",
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
			"templates": []string{"{schema}://{apikey}@{from_phone}", "{schema}://{apikey}@{from_phone}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "httpsms",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"httpsms"},
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
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"httpsms"},
		"service_name":     "httpSMS",
		"service_url":      "https://httpsms.com",
		"setup_url":        "https://appriseit.com/services/httpsms/",
	})
}
