package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const iftttNotifyURL = "https://maker.ifttt.com/trigger/%s/with/key/%s"

type IFTTTTarget struct {
	webhookID string
	events    []string
	addTokens map[string]string
	delTokens map[string]struct{}
}

func NewIFTTTTarget(target *ParsedURL) (*IFTTTTarget, error) {
	webhookID := target.Host
	if target.User != "" {
		webhookID = target.User
	}
	if webhookID == "" {
		return nil, fmt.Errorf("missing webhook id")
	}

	events := []string{}
	if target.User != "" && target.Host != "" {
		events = append(events, target.Host)
	}
	events = append(events, splitPath(target.Path)...)
	if rawEvents, ok := target.Query["to"]; ok && rawEvents != "" {
		events = append(events, splitList(rawEvents)...)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("missing events")
	}

	return &IFTTTTarget{
		webhookID: webhookID,
		events:    events,
		addTokens: map[string]string{},
		delTokens: map[string]struct{}{},
	}, nil
}

func (t *IFTTTTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(t.events) == 0 {
		return RequestSpec{}, fmt.Errorf("missing events")
	}

	return t.buildRequestForEvent(t.events[0], body, title, notifyType)
}

func (t *IFTTTTarget) Send(body, title string, notifyType NotifyType) error {
	for _, event := range t.events {
		spec, err := t.buildRequestForEvent(event, body, title, notifyType)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (t *IFTTTTarget) buildRequestForEvent(event, body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"value1": title,
		"value2": body,
		"value3": string(notifyType),
	}

	for key, value := range t.addTokens {
		payload[key] = value
	}

	normalized := map[string]any{}
	for key, value := range payload {
		lowerKey := strings.ToLower(key)
		if _, dropped := t.delTokens[lowerKey]; dropped {
			continue
		}
		normalized[lowerKey] = value
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}

	return RequestSpec{
		Method:  "POST",
		URL:     fmt.Sprintf(iftttNotifyURL, event, t.webhookID),
		Headers: headers,
		Body:    string(data),
	}, nil
}

func splitList(raw string) []string {
	out := []string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func init() {
	RegisterSchemaEntryOrdered(125, SchemaEntry{
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
					"alias_of": "events",
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
			"kwargs": map[string]any{
				"add_tokens": map[string]any{
					"map_to":   "add_tokens",
					"name":     "Add Tokens",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"del_tokens": map[string]any{
					"map_to":   "del_tokens",
					"name":     "Remove Tokens",
					"prefix":   "-",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{webhook_id}/{events}"},
			"tokens": map[string]any{
				"events": map[string]any{
					"delim":    []string{"/"},
					"group":    []any{},
					"map_to":   "events",
					"name":     "Events",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"schema": map[string]any{
					"default":  "ifttt",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"ifttt"},
				},
				"webhook_id": map[string]any{
					"map_to":   "webhook_id",
					"name":     "Webhook ID",
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
		"secure_protocols": []string{"ifttt"},
		"service_name":     "IFTTT",
		"service_url":      "https://ifttt.com/",
		"setup_url":        "https://appriseit.com/services/ifttt/",
	})
}
