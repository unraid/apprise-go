package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const smsManagerURL = "https://http-api.smsmanager.cz/Send"
const smsManagerBatchSize = 4000

var smsManagerGateways = map[string]struct{}{
	"high":    {},
	"economy": {},
	"low":     {},
	"direct":  {},
}

type SMSManagerTarget struct {
	apiKey  string
	sender  string
	gateway string
	batch   bool
	targets []string
}

func NewSMSManagerTarget(target *ParsedURL) (*SMSManagerTarget, error) {
	apiKey := strings.TrimSpace(target.User)
	if rawKey, ok := target.Query["key"]; ok && rawKey != "" {
		apiKey = rawKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	sender := ""
	if rawSender, ok := target.Query["from"]; ok && rawSender != "" {
		sender = rawSender
	} else if rawSender, ok := target.Query["sender"]; ok && rawSender != "" {
		sender = rawSender
	}
	sender = strings.TrimSpace(sender)
	if sender != "" && len(sender) > 11 {
		sender = sender[:11]
	}

	gateway := strings.ToLower(strings.TrimSpace(target.Query["gateway"]))
	if gateway == "" {
		gateway = "high"
	}
	if _, ok := smsManagerGateways[gateway]; !ok {
		return nil, fmt.Errorf("invalid gateway")
	}

	batch := parseBool(target.Query["batch"], false)

	entries := []string{}
	if target.Host != "" {
		entries = append(entries, target.Host)
	}
	entries = append(entries, splitPath(target.Path)...)
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	targets := []string{}
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if normalized, ok := normalizePhoneWithBounds(trimmed, 9, 14); ok {
			if strings.HasPrefix(trimmed, "+") {
				targets = append(targets, "+"+normalized)
			} else {
				targets = append(targets, normalized)
			}
		}
	}

	return &SMSManagerTarget{
		apiKey:  apiKey,
		sender:  sender,
		gateway: gateway,
		batch:   batch,
		targets: targets,
	}, nil
}

func (s *SMSManagerTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(s.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	numbers := s.targets[:1]
	if s.batch {
		numbers = s.targets[:minInt(len(s.targets), smsManagerBatchSize)]
	}
	payload := s.buildPayload(message, numbers)

	requestURL := smsManagerURL
	if encoded := payload.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}

	_ = notifyType

	return RequestSpec{
		Method: "GET",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent": "Apprise",
			"Accept":     "*/*",
		},
	}, nil
}

func (s *SMSManagerTarget) Send(body, title string, notifyType NotifyType) error {
	if len(s.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if s.batch {
		batchSize = smsManagerBatchSize
	}

	for index := 0; index < len(s.targets); index += batchSize {
		end := index + batchSize
		if end > len(s.targets) {
			end = len(s.targets)
		}
		payload := s.buildPayload(message, s.targets[index:end])
		requestURL := smsManagerURL
		if encoded := payload.Encode(); encoded != "" {
			requestURL += "?" + encoded
		}

		spec := RequestSpec{
			Method: "GET",
			URL:    requestURL,
			Headers: map[string]string{
				"User-Agent": "Apprise",
				"Accept":     "*/*",
			},
		}

		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (s *SMSManagerTarget) buildPayload(message string, targets []string) url.Values {
	payload := url.Values{}
	payload.Set("apikey", s.apiKey)
	payload.Set("gateway", s.gateway)
	payload.Set("message", message)
	payload.Set("number", strings.Join(targets, ";"))
	if s.sender != "" {
		payload.Set("sender", s.sender)
	}
	return payload
}

func init() {
	RegisterSchemaEntryOrdered(54, SchemaEntry{
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
					"map_to":   "sender",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"gateway": map[string]any{
					"default":  "high",
					"map_to":   "gateway",
					"name":     "Gateway",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"high", "economy", "low", "direct"},
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
				"sender": map[string]any{
					"alias_of": "from",
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
			"templates": []string{"{schema}://{apikey}@{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"smsmanager", "smsmgr"},
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
		"secure_protocols": []string{"smsmgr", "smsmanager"},
		"service_name":     "SMS Manager",
		"service_url":      "https://smsmanager.cz",
		"setup_url":        "https://appriseit.com/services/sms_manager/",
	})
}
