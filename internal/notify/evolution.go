package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

type EvolutionTarget struct {
	apikey   string
	host     string
	port     int
	secure   bool
	instance string
	phones   []string
}

func NewEvolutionTarget(target *ParsedURL) (*EvolutionTarget, error) {
	apikey := strings.TrimSpace(target.User)
	if apikey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	entries := splitPath(target.Path)
	if len(entries) == 0 {
		return nil, fmt.Errorf("missing instance")
	}
	instance := entries[0]
	entries = entries[1:]

	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	phones := make([]string, 0, len(entries))
	for _, entry := range entries {
		if normalized, ok := normalizePhone(entry); ok {
			phones = append(phones, normalized)
		}
	}
	if len(phones) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &EvolutionTarget{
		apikey:   apikey,
		host:     host,
		port:     target.Port,
		secure:   strings.EqualFold(target.Scheme, "evolutions"),
		instance: instance,
		phones:   phones,
	}, nil
}

func (e *EvolutionTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := e.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (e *EvolutionTarget) Send(body, title string, notifyType NotifyType) error {
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

// buildRequests sends one message per number through the instance endpoint.
func (e *EvolutionTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	scheme := "http"
	defaultPort := 80
	if e.secure {
		scheme = "https"
		defaultPort = 443
	}

	base := fmt.Sprintf("%s://%s", scheme, e.host)
	// The default port for the scheme is left implicit.
	if e.port > 0 && e.port != defaultPort {
		base += fmt.Sprintf(":%d", e.port)
	}

	endpoint := fmt.Sprintf("%s/message/sendText/%s", base, e.instance)
	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
		"apikey":       e.apikey,
	}

	specs := make([]RequestSpec, 0, len(e.phones))
	for _, number := range e.phones {
		data, err := json.Marshal(map[string]any{
			"number": number,
			// WhatsApp has its own markdown dialect, not CommonMark.
			"text": commonMarkToWhatsApp(mergeTitleBody(title, body)),
		})
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     endpoint,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}

func init() {
	RegisterSchemaEntryOrdered(158, SchemaEntry{
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
					"default":  "markdown",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
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
			"templates": []string{"{schema}://{apikey}@{host}/{instance}/{targets}", "{schema}://{apikey}@{host}:{port}/{instance}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"instance": map[string]any{
					"map_to":   "instance",
					"name":     "Instance Name",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"port": map[string]any{
					"map_to":   "port",
					"max":      65535,
					"min":      1,
					"name":     "Port",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"evolution", "evolutions"},
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
		"protocols": []string{"evolution"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"evolutions"},
		"service_name":     "Evolution API",
		"service_url":      "https://github.com/EvolutionAPI/evolution-api",
		"setup_url":        "https://appriseit.com/services/evolution/",
	})
}
