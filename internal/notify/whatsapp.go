package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const whatsappGraphVersion = "v17.0"
const whatsappURLTemplate = "https://graph.facebook.com/" + whatsappGraphVersion + "/%s/messages"
const whatsappDefaultLanguage = "en_US"

var whatsappComponentKey = regexp.MustCompile(`(?i)^([1-9][0-9]*|body|type)$`)

type WhatsAppTarget struct {
	token      string
	fromID     string
	template   string
	language   string
	targets    []string
	components map[int]whatsappComponent
	order      []int
}

type whatsappComponent struct {
	manual bool
	text   string
	mapTo  string
}

func NewWhatsAppTarget(target *ParsedURL) (*WhatsAppTarget, error) {
	token := strings.TrimSpace(target.User)
	template := ""
	if target.Password != "" {
		template = token
		token = target.Password
	}

	if raw := strings.TrimSpace(target.Query["token"]); raw != "" {
		token = raw
	}
	if raw := strings.TrimSpace(target.Query["template"]); raw != "" {
		template = raw
	}

	fromID := strings.TrimSpace(target.Host)
	if raw := strings.TrimSpace(target.Query["from"]); raw != "" {
		fromID = raw
	} else if raw := strings.TrimSpace(target.Query["source"]); raw != "" {
		fromID = raw
	}
	if token == "" || fromID == "" {
		return nil, fmt.Errorf("missing token or from id")
	}

	language := ""
	if template != "" {
		language = whatsappDefaultLanguage
		if raw := strings.TrimSpace(target.Query["lang"]); raw != "" {
			language = raw
		}
	}

	rawTargets := splitPath(target.Path)
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		rawTargets = append(rawTargets, parseDelimitedList(toValue)...)
	}

	targets := []string{}
	for _, entry := range rawTargets {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if normalized, ok := normalizePhone(entry); ok {
			targets = append(targets, "+"+normalized)
		}
	}

	components, order, err := parseWhatsAppComponents(target.QueryPayload)
	if err != nil {
		return nil, err
	}

	return &WhatsAppTarget{
		token:      token,
		fromID:     fromID,
		template:   template,
		language:   language,
		targets:    targets,
		components: components,
		order:      order,
	}, nil
}

func (w *WhatsAppTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(w.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	payload := w.buildPayload(message, notifyType, w.targets[0])

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    fmt.Sprintf(whatsappURLTemplate, w.fromID),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "application/json",
			"Content-Type": "application/json",
			"Authorization": fmt.Sprintf(
				"Bearer %s",
				w.token,
			),
		},
		Body: string(data),
	}, nil
}

func (w *WhatsAppTarget) Send(body, title string, notifyType NotifyType) error {
	if len(w.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)

	for _, target := range w.targets {
		payload := w.buildPayload(message, notifyType, target)
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    fmt.Sprintf(whatsappURLTemplate, w.fromID),
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "application/json",
				"Content-Type": "application/json",
				"Authorization": fmt.Sprintf(
					"Bearer %s",
					w.token,
				),
			},
			Body: string(data),
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (w *WhatsAppTarget) buildPayload(message string, notifyType NotifyType, target string) map[string]any {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                target,
	}

	if w.template == "" {
		payload["recipient_type"] = "individual"
		payload["type"] = "text"
		payload["text"] = map[string]string{
			"body": message,
		}
		return payload
	}

	template := map[string]any{
		"name":     w.template,
		"language": map[string]string{"code": w.language},
	}

	if len(w.components) > 0 {
		parameters := make([]map[string]string, 0, len(w.components))
		for _, key := range w.order {
			component := w.components[key]
			if component.manual {
				parameters = append(parameters, map[string]string{
					"type": "text",
					"text": component.text,
				})
				continue
			}

			value := string(notifyType)
			if component.mapTo == "body" {
				value = message
			}

			parameters = append(parameters, map[string]string{
				"type": "text",
				"text": value,
			})
		}

		template["components"] = []any{
			map[string]any{
				"type":       "body",
				"parameters": parameters,
			},
		}
	}

	payload["type"] = "template"
	payload["template"] = template

	return payload
}

func parseWhatsAppComponents(payload map[string]string) (map[int]whatsappComponent, []int, error) {
	if len(payload) == 0 {
		return nil, nil, nil
	}

	components := map[int]whatsappComponent{}
	order := []int{}

	for key, value := range payload {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if !whatsappComponentKey.MatchString(key) {
			return nil, nil, fmt.Errorf("invalid component key")
		}

		if key == "body" || key == "type" {
			if !whatsappComponentKey.MatchString(value) || value == "" {
				return nil, nil, fmt.Errorf("invalid component mapping")
			}
			if value == "body" || value == "type" {
				return nil, nil, fmt.Errorf("invalid component mapping")
			}
			index, err := parseComponentIndex(value)
			if err != nil {
				return nil, nil, err
			}
			if _, exists := components[index]; exists {
				return nil, nil, fmt.Errorf("duplicate component index")
			}
			components[index] = whatsappComponent{mapTo: key}
			order = append(order, index)
			continue
		}

		index, err := parseComponentIndex(key)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := components[index]; exists {
			return nil, nil, fmt.Errorf("duplicate component index")
		}
		components[index] = whatsappComponent{
			manual: true,
			text:   value,
		}
		order = append(order, index)
	}

	sort.Ints(order)
	return components, order, nil
}

func parseComponentIndex(raw string) (int, error) {
	value := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid component index")
		}
		value = value*10 + int(r-'0')
	}
	if value <= 0 {
		return 0, fmt.Errorf("invalid component index")
	}
	return value, nil
}

func init() {
	RegisterSchemaEntryOrdered(20, SchemaEntry{
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
					"alias_of": "from_phone_id",
				},
				"lang": map[string]any{
					"alias_of": "language",
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
				"template": map[string]any{
					"alias_of": "template",
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
			},
			"kwargs": map[string]any{
				"template_mapping": map[string]any{
					"map_to":   "template_mapping",
					"name":     "Template Mapping",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{token}@{from_phone_id}/{targets}", "{schema}://{template}:{token}@{from_phone_id}/{targets}"},
			"tokens": map[string]any{
				"from_phone_id": map[string]any{
					"map_to":   "from_phone_id",
					"name":     "From Phone ID",
					"private":  true,
					"regex":    []string{"^[0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"language": map[string]any{
					"default":  "en_US",
					"map_to":   "language",
					"name":     "Language",
					"private":  false,
					"regex":    []string{"^[^0-9\\s]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "whatsapp",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"whatsapp"},
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
				"template": map[string]any{
					"map_to":   "template",
					"name":     "Template Name",
					"private":  false,
					"regex":    []string{"^[^\\s]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Access Token",
					"private":  true,
					"regex":    []string{"^[a-z0-9]+$", "i"},
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
		"secure_protocols": []string{"whatsapp"},
		"service_name":     "WhatsApp",
		"service_url":      "https://developers.facebook.com/docs/whatsapp/cloud-api/get-started",
		"setup_url":        "https://appriseit.com/services/whatsapp/",
	})
}
