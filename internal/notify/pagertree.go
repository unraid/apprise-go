package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var pagertreeIntegrationRegex = regexp.MustCompile(`^int_[A-Za-z0-9_-]{7,14}$`)

const pagertreeFixedUUID = "00000000-0000-4000-8000-000000000000"

type PagerTreeTarget struct {
	integration string
	action      string
	thirdparty  string
	urgency     string
	tags        []string
	headers     map[string]string
	payload     map[string]string
	meta        map[string]string
}

func NewPagerTreeTarget(target *ParsedURL) (*PagerTreeTarget, error) {
	integration := strings.TrimSpace(target.Host)
	if integration == "" {
		return nil, fmt.Errorf("missing integration id")
	}
	if !pagertreeIntegrationRegex.MatchString(integration) {
		return nil, fmt.Errorf("invalid integration id")
	}

	action := strings.ToLower(strings.TrimSpace(target.Query["action"]))
	if action == "" {
		action = "create"
	}
	switch action {
	case "create", "acknowledge", "resolve":
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}

	thirdparty := strings.TrimSpace(target.Query["thirdparty"])
	if thirdparty == "" {
		thirdparty = strings.TrimSpace(target.Query["tid"])
	}

	urgency := strings.TrimSpace(target.Query["urgency"])

	tags := []string{}
	if tagValue, ok := target.Query["tags"]; ok && strings.TrimSpace(tagValue) != "" {
		tags = append(tags, parseDelimitedList(tagValue)...)
	}

	headers := map[string]string{}
	for key, value := range target.QueryAdd {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		headers[key] = value
	}

	payloadExtras := map[string]string{}
	for key, value := range target.QueryPayload {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		payloadExtras[key] = value
	}

	metaExtras := map[string]string{}
	for key, value := range target.QueryDel {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		metaExtras[key] = value
	}

	return &PagerTreeTarget{
		integration: integration,
		action:      action,
		thirdparty:  thirdparty,
		urgency:     urgency,
		tags:        tags,
		headers:     headers,
		payload:     payloadExtras,
		meta:        metaExtras,
	}, nil
}

func (p *PagerTreeTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (p *PagerTreeTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	thirdparty := p.thirdparty
	if thirdparty == "" {
		thirdparty = pagertreeFixedUUID
	}
	payload := map[string]any{
		"id":         thirdparty,
		"event_type": p.action,
	}

	if p.action == "create" {
		eventTitle := title
		if strings.TrimSpace(eventTitle) == "" {
			eventTitle = "Apprise Notifications"
		}

		meta := map[string]any{}
		for key, value := range p.meta {
			meta[key] = value
		}
		payload["title"] = eventTitle
		payload["description"] = body
		payload["meta"] = meta
		payload["tags"] = p.tags
		if p.urgency != "" {
			payload["urgency"] = p.urgency
		}
	}

	for key, value := range p.payload {
		payload[key] = value
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	url := fmt.Sprintf("https://api.pagertree.com/integration/%s", p.integration)

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}
	for key, value := range p.headers {
		headers[key] = value
	}

	return RequestSpec{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(12, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"action": map[string]any{
					"default":  "create",
					"map_to":   "action",
					"name":     "Action",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"create", "acknowledge", "resolve"},
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
				"tags": map[string]any{
					"map_to":   "tags",
					"name":     "Tags",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"thirdparty": map[string]any{
					"map_to":   "thirdparty",
					"name":     "Third Party ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"urgency": map[string]any{
					"map_to":   "urgency",
					"name":     "Urgency",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"silent", "low", "medium", "high", "critical"},
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
				"headers": map[string]any{
					"map_to":   "headers",
					"name":     "HTTP Header",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"meta_extras": map[string]any{
					"map_to":   "meta_extras",
					"name":     "Meta Extras",
					"prefix":   "-",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"payload_extras": map[string]any{
					"map_to":   "payload_extras",
					"name":     "Payload Extras",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{integration}"},
			"tokens": map[string]any{
				"integration": map[string]any{
					"map_to":   "integration",
					"name":     "Integration ID",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "pagertree",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pagertree"},
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
		"secure_protocols": []string{"pagertree"},
		"service_name":     "PagerTree",
		"service_url":      "https://pagertree.com/",
		"setup_url":        "https://appriseit.com/services/pagertree/",
	})
}
