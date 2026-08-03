package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const eight00comURL = "https://api.800.com/message"

type Eight00comTarget struct {
	token   string
	source  string
	targets []string
}

func NewEight00comTarget(target *ParsedURL) (*Eight00comTarget, error) {
	token := strings.TrimSpace(target.Query["token"])
	if token == "" {
		token = strings.TrimSpace(target.User)
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	// ?from= names the sender and frees the host to be a recipient;
	// otherwise the host is the sender.
	var source string
	var entries []string
	if raw := strings.TrimSpace(target.Query["from"]); raw != "" {
		source = raw
		entries = append([]string{strings.TrimSpace(target.Host)}, splitPath(target.Path)...)
	} else {
		source = strings.TrimSpace(target.Host)
		entries = splitPath(target.Path)
	}

	normalizedSource, ok := normalizePhone(source)
	if !ok {
		return nil, fmt.Errorf("invalid source phone number")
	}

	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	targets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if normalized, valid := normalizePhone(entry); valid {
			targets = append(targets, normalized)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &Eight00comTarget{
		token:   token,
		source:  normalizedSource,
		targets: targets,
	}, nil
}

func (e *Eight00comTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := e.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (e *Eight00comTarget) Send(body, title string, notifyType NotifyType) error {
	specs, err := e.buildRequests(body, title, notifyType)
	if err != nil {
		return err
	}

	for _, spec := range specs {
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

// buildRequests sends one message per recipient.
func (e *Eight00comTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + e.token,
	}

	specs := make([]RequestSpec, 0, len(e.targets))
	for _, recipient := range e.targets {
		data, err := json.Marshal(map[string]any{
			"sender": "+" + e.source,
			// Both numbers travel in E.164 with the plus retained.
			"recipient": "+" + recipient,
			"message":   mergeTitleBody(title, body),
		})
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     eight00comURL,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}

func init() {
	RegisterSchemaEntryOrdered(155, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"cto": map[string]any{
					"default":  4.0,
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
					"required": false,
					"type":     "string",
				},
				"optional": map[string]any{
					"default":  false,
					"map_to":   "optional",
					"name":     "Optional Service",
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
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"retry": map[string]any{
					"default":  0,
					"map_to":   "retry",
					"max":      10,
					"min":      0,
					"name":     "Service Retry",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"rto": map[string]any{
					"default":  4.0,
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
				"token": map[string]any{
					"alias_of": "token",
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
				"wait": map[string]any{
					"default":  0.0,
					"map_to":   "wait",
					"max":      20.0,
					"min":      0.0,
					"name":     "Inter-Retry Wait",
					"private":  false,
					"required": false,
					"type":     "float",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}@{from_phone}", "{schema}://{token}@{from_phone}/{targets}"},
			"tokens": map[string]any{
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "eight00com",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"eight00com"},
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
				"token": map[string]any{
					"map_to":   "token",
					"name":     "API Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled": true,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"protocols":        nil,
		"secure_protocols": []string{"eight00com"},
		"service_name":     "800.com",
		"service_url":      "https://www.800.com",
		"setup_url":        "https://appriseit.com/services/eight00com/",
	})
}
