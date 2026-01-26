package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const kumulosNotifyURL = "https://messages.kumulos.com/v2/notifications"

type KumulosTarget struct {
	apiKey    string
	serverKey string
}

func NewKumulosTarget(target *ParsedURL) (*KumulosTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if apiKey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	parts := splitPath(target.Path)
	if len(parts) == 0 {
		return nil, fmt.Errorf("missing server key")
	}
	serverKey := strings.TrimSpace(parts[0])
	if serverKey == "" {
		return nil, fmt.Errorf("missing server key")
	}

	return &KumulosTarget{
		apiKey:    apiKey,
		serverKey: serverKey,
	}, nil
}

func (k *KumulosTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"target": map[string]any{
			"broadcast": true,
		},
		"content": map[string]any{
			"title":   title,
			"message": body,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    kumulosNotifyURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": basicAuthHeader(k.apiKey, k.serverKey),
		},
		Body: string(data),
	}, nil
}

func (k *KumulosTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := k.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(22, SchemaEntry{
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
			"templates": []string{"{schema}://{apikey}/{serverkey}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "kumulos",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"kumulos"},
				},
				"serverkey": map[string]any{
					"map_to":   "serverkey",
					"name":     "Server Key",
					"private":  true,
					"regex":    []string{"^[A-Z0-9+]{36}$", "i"},
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
		"secure_protocols": []string{"kumulos"},
		"service_name":     "Kumulos",
		"service_url":      "https://kumulos.com/",
		"setup_url":        "https://appriseit.com/services/kumulos/",
	})
}
