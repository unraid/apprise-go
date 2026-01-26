package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const clicksendURL = "https://rest.clicksend.com/v3/sms/send"
const clicksendBatchSize = 1000

type ClickSendTarget struct {
	user     string
	password string
	targets  []string
	batch    bool
}

func NewClickSendTarget(target *ParsedURL) (*ClickSendTarget, error) {
	user := strings.TrimSpace(target.User)
	password := target.Password
	if rawKey, ok := target.Query["key"]; ok && rawKey != "" {
		password = rawKey
	}
	if user == "" || password == "" {
		return nil, fmt.Errorf("missing credentials")
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

	batch := parseBool(target.Query["batch"], false)

	return &ClickSendTarget{
		user:     user,
		password: password,
		targets:  targets,
		batch:    batch,
	}, nil
}

func (c *ClickSendTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(c.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if c.batch {
		batchSize = clicksendBatchSize
	}
	spec, err := c.buildRequest(c.targets[:min(len(c.targets), batchSize)], message)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (c *ClickSendTarget) Send(body, title string, notifyType NotifyType) error {
	if len(c.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if c.batch {
		batchSize = clicksendBatchSize
	}

	for index := 0; index < len(c.targets); index += batchSize {
		end := index + batchSize
		if end > len(c.targets) {
			end = len(c.targets)
		}
		spec, err := c.buildRequest(c.targets[index:end], message)
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

func (c *ClickSendTarget) buildRequest(targets []string, message string) (RequestSpec, error) {
	messages := make([]map[string]string, 0, len(targets))
	for _, target := range targets {
		messages = append(messages, map[string]string{
			"source": "php",
			"body":   message,
			"to":     "+" + target,
		})
	}

	payload := map[string]any{
		"messages": messages,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    clicksendURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Content-Type":  "application/json; charset=utf-8",
			"Authorization": basicAuthHeader(c.user, c.password),
		},
		Body: string(data),
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	RegisterSchemaEntryOrdered(96, SchemaEntry{
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
			"templates": []string{"{schema}://{user}:{apikey}@{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "password",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "clicksend",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"clicksend"},
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
				"user": map[string]any{
					"map_to":   "user",
					"name":     "User Name",
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
		"secure_protocols": []string{"clicksend"},
		"service_name":     "ClickSend",
		"service_url":      "https://clicksend.com/",
		"setup_url":        "https://appriseit.com/services/clicksend/",
	})
}
