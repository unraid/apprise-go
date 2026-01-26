package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const d7NetworksURL = "https://api.d7networks.com/messages/v1/send"

type D7NetworksTarget struct {
	token   string
	targets []string
	source  string
	batch   bool
	unicode bool
}

func NewD7NetworksTarget(target *ParsedURL) (*D7NetworksTarget, error) {
	token := ""
	if rawToken, ok := target.Query["token"]; ok && rawToken != "" {
		token = rawToken
	} else if target.User != "" {
		token = target.User
		if target.Password != "" {
			token += ":" + target.Password
		}
	} else if target.Password != "" {
		token = ":" + target.Password
	}

	if token == "" {
		return nil, fmt.Errorf("missing token")
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

	appendTarget(target.Host)
	for _, entry := range splitPath(target.Path) {
		appendTarget(entry)
	}
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			appendTarget(entry)
		}
	}

	source := ""
	if rawSource, ok := target.Query["from"]; ok && rawSource != "" {
		source = rawSource
	} else if rawSource, ok := target.Query["source"]; ok && rawSource != "" {
		source = rawSource
	}
	source = strings.TrimSpace(source)

	batch := parseBool(target.Query["batch"], false)
	unicode := parseBool(target.Query["unicode"], false)

	return &D7NetworksTarget{
		token:   token,
		targets: targets,
		source:  source,
		batch:   batch,
		unicode: unicode,
	}, nil
}

func (d *D7NetworksTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(d.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	recipients := []string{d.targets[0]}
	if d.batch {
		recipients = d.targets
	}

	spec, err := d.buildRequest(recipients, message)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (d *D7NetworksTarget) Send(body, title string, notifyType NotifyType) error {
	if len(d.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	if d.batch {
		spec, err := d.buildRequest(d.targets, message)
		if err != nil {
			return err
		}
		return SendRequest(spec)
	}

	for _, target := range d.targets {
		spec, err := d.buildRequest([]string{target}, message)
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

func (d *D7NetworksTarget) buildRequest(recipients []string, message string) (RequestSpec, error) {
	dataCoding := "auto"
	if d.unicode {
		dataCoding = "unicode"
	}

	messageGlobals := map[string]any{
		"channel": "sms",
	}
	if d.source != "" {
		messageGlobals["originator"] = d.source
	}

	payload := map[string]any{
		"message_globals": messageGlobals,
		"messages": []map[string]any{
			{
				"recipients":  recipients,
				"content":     message,
				"data_coding": dataCoding,
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    d7NetworksURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "application/json",
			"Content-Type":  "application/json",
			"Authorization": fmt.Sprintf("Bearer %s", d.token),
		},
		Body: string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(86, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"batch": map[string]any{
					"default":  false,
					"map_to":   "batch",
					"name":     "Batch Mode",
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
				"unicode": map[string]any{
					"default":  false,
					"map_to":   "unicode",
					"name":     "Unicode Characters",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			"templates": []string{"{schema}://{token}@{targets}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "d7sms",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"d7sms"},
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
				"token": map[string]any{
					"map_to":   "token",
					"name":     "API Access Token",
					"private":  true,
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
		"secure_protocols": []string{"d7sms"},
		"service_name":     "D7 Networks",
		"service_url":      "https://d7networks.com/",
		"setup_url":        "https://appriseit.com/services/d7networks/",
	})
}
