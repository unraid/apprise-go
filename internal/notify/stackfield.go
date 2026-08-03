package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const stackfieldURLTemplate = "https://www.stackfield.com/apiwh/%s"

type StackfieldTarget struct {
	token string
}

func NewStackfieldTarget(target *ParsedURL) (*StackfieldTarget, error) {
	// The token is the host, or ?token= which is easier to express in YAML.
	token := strings.TrimSpace(target.Query["token"])
	if token == "" {
		token = strings.TrimSpace(target.Host)
	}
	if token == "" {
		return nil, fmt.Errorf("missing webhook token")
	}

	return &StackfieldTarget{token: token}, nil
}

func (s *StackfieldTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	_ = notifyType

	// Stackfield has no separate title field, so the title is folded into the
	// message the way the framework does upstream.
	payload := map[string]any{"Title": mergeTitleBody(title, body)}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    fmt.Sprintf(stackfieldURLTemplate, s.token),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

// Send posts the notification to the incoming webhook.
func (s *StackfieldTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := s.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(141, SchemaEntry{
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
			"templates": []string{"{schema}://{token}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "stackfield",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"stackfield"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Webhook Token",
					"private":  true,
					"regex":    []string{"^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$", "i"},
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
		"secure_protocols": []string{"stackfield"},
		"service_name":     "Stackfield",
		"service_url":      "https://www.stackfield.com/",
		"setup_url":        "https://appriseit.com/services/stackfield/",
	})
}
