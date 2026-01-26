package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const clickatellURL = "https://platform.clickatell.com/messages/http/send"

type ClickatellTarget struct {
	apiKey  string
	source  string
	targets []string
}

func NewClickatellTarget(target *ParsedURL) (*ClickatellTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	source := strings.TrimSpace(target.User)
	if rawSource, ok := target.Query["from"]; ok && rawSource != "" {
		source = strings.TrimSpace(rawSource)
	}
	if source != "" {
		normalized, ok := normalizePhone(source)
		if !ok {
			return nil, fmt.Errorf("invalid source")
		}
		source = normalized
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

	return &ClickatellTarget{
		apiKey:  apiKey,
		source:  source,
		targets: targets,
	}, nil
}

func (c *ClickatellTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(c.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	spec, err := c.buildRequest(c.targets[0], message)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (c *ClickatellTarget) Send(body, title string, notifyType NotifyType) error {
	if len(c.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	for _, target := range c.targets {
		spec, err := c.buildRequest(target, message)
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

func (c *ClickatellTarget) buildRequest(target, message string) (RequestSpec, error) {
	params := url.Values{}
	params.Set("apiKey", c.apiKey)
	params.Set("content", message)
	params.Set("to", target)
	if c.source != "" {
		params.Set("from", c.source)
	}

	requestURL := clickatellURL
	if encoded := params.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}

	return RequestSpec{
		Method: "GET",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "application/json",
			"Content-Type": "application/json",
		},
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(10, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"apikey": map[string]any{
					"alias_of": "apikey",
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
			"templates": []string{"{schema}://{apikey}/{targets}", "{schema}://{source}@{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "clickatell",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"clickatell"},
				},
				"source": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^[0-9\\s)(+-]+$", "i"},
					"required": false,
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
		"secure_protocols": []string{"clickatell"},
		"service_name":     "Clickatell",
		"service_url":      "https://www.clickatell.com/",
		"setup_url":        "https://appriseit.com/services/clickatell/",
	})
}
