package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const splunkURL = "https://alert.victorops.com/integrations/generic/20131114/alert/%s/%s"

var splunkTokenRe = regexp.MustCompile(`(?i)^[A-Z0-9_-]+$`)

var splunkActionOrder = []string{
	"map",
	"info",
	"acknowledgement",
	"warning",
	"recovery",
	"resolve",
	"critical",
}

var splunkMessageTypes = []string{
	"CRITICAL",
	"WARNING",
	"ACKNOWLEDGEMENT",
	"INFO",
	"RECOVERY",
}

type SplunkTarget struct {
	apiKey     string
	routingKey string
	entityID   string
	action     string
	mapping    map[NotifyType]string
}

func NewSplunkTarget(target *ParsedURL) (*SplunkTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if rawAPI := strings.TrimSpace(target.Query["apikey"]); rawAPI != "" {
		apiKey = rawAPI
	}
	if apiKey == "" || !splunkTokenRe.MatchString(apiKey) {
		return nil, fmt.Errorf("invalid apikey")
	}

	routingKey := strings.TrimSpace(target.User)
	if rawRoute := strings.TrimSpace(target.Query["routing_key"]); rawRoute != "" {
		routingKey = rawRoute
	} else if rawRoute := strings.TrimSpace(target.Query["route"]); rawRoute != "" {
		routingKey = rawRoute
	}
	if routingKey == "" || !splunkTokenRe.MatchString(routingKey) {
		return nil, fmt.Errorf("invalid routing key")
	}

	entityID := strings.TrimSpace(target.Query["entity_id"])
	if entityID == "" {
		entityID = strings.TrimSpace(target.Path)
		entityID = strings.Trim(entityID, " \r\n\t\v/")
	}
	if entityID == "" {
		entityID = "Apprise/" + routingKey
	}

	action := normalizeSplunkAction(target.Query["action"])
	if action == "" {
		action = "map"
	}

	mapping := map[NotifyType]string{
		NotifyInfo:    "INFO",
		NotifySuccess: "RECOVERY",
		NotifyWarning: "WARNING",
		NotifyFailure: "CRITICAL",
	}

	for key, value := range target.QueryPayload {
		notifyType, ok := normalizeSplunkNotifyType(key)
		if !ok {
			return nil, fmt.Errorf("invalid mapping key")
		}
		messageType := normalizeSplunkMessageType(value)
		if messageType == "" {
			return nil, fmt.Errorf("invalid mapping value")
		}
		mapping[notifyType] = messageType
	}

	return &SplunkTarget{
		apiKey:     apiKey,
		routingKey: routingKey,
		entityID:   entityID,
		action:     action,
		mapping:    mapping,
	}, nil
}

func (s *SplunkTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	messageType := s.resolveMessageType(notifyType)
	payload := map[string]any{
		"entity_id":           s.entityID,
		"message_type":        messageType,
		"entity_display_name": splunkEntityDisplayName(title),
		"state_message":       body,
		"monitoring_tool":     "Apprise",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	requestURL := fmt.Sprintf(splunkURL, s.apiKey, s.routingKey)
	return RequestSpec{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (s *SplunkTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := s.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (s *SplunkTarget) resolveMessageType(notifyType NotifyType) string {
	switch s.action {
	case "acknowledgement":
		return "ACKNOWLEDGEMENT"
	case "info":
		return "INFO"
	case "critical":
		return "CRITICAL"
	case "warning":
		return "WARNING"
	case "recovery", "resolve":
		return "RECOVERY"
	default:
		if messageType, ok := s.mapping[notifyType]; ok {
			return messageType
		}
		return "INFO"
	}
}

func splunkEntityDisplayName(title string) string {
	if strings.TrimSpace(title) == "" {
		return "Apprise Notifications"
	}
	return title
}

func normalizeSplunkAction(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	for _, action := range splunkActionOrder {
		if strings.HasPrefix(action, raw) {
			return action
		}
	}
	return ""
}

func normalizeSplunkMessageType(raw string) string {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	for _, messageType := range splunkMessageTypes {
		if strings.HasPrefix(messageType, raw) {
			return messageType
		}
	}
	return ""
}

func normalizeSplunkNotifyType(raw string) (NotifyType, bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(string(NotifyInfo), raw):
		return NotifyInfo, true
	case strings.HasPrefix(string(NotifySuccess), raw):
		return NotifySuccess, true
	case strings.HasPrefix(string(NotifyWarning), raw):
		return NotifyWarning, true
	case strings.HasPrefix(string(NotifyFailure), raw):
		return NotifyFailure, true
	default:
		return NotifyInfo, false
	}
}

func init() {
	RegisterSchemaEntryOrdered(119, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"action": map[string]any{
					"default":  "map",
					"map_to":   "action",
					"name":     "Action",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"map", "info", "acknowledgement", "warning", "recovery", "resolve", "critical"},
				},
				"apikey": map[string]any{
					"alias_of": "apikey",
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
				"entity_id": map[string]any{
					"alias_of": "entity_id",
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
				"route": map[string]any{
					"alias_of": "routing_key",
				},
				"routing_key": map[string]any{
					"alias_of": "routing_key",
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
				"mapping": map[string]any{
					"map_to":   "mapping",
					"name":     "Action Mapping",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{routing_key}@{apikey}", "{schema}://{routing_key}@{apikey}/{entity_id}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[A-Z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"entity_id": map[string]any{
					"map_to":   "entity_id",
					"name":     "Entity ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"routing_key": map[string]any{
					"map_to":   "routing_key",
					"name":     "Target Routing Key",
					"private":  false,
					"regex":    []string{"^[A-Z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"splunk", "victorops"},
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
		"secure_protocols": []string{"splunk", "victorops"},
		"service_name":     "Splunk On-Call",
		"service_url":      "https://www.splunk.com/en_us/products/on-call.html",
		"setup_url":        "https://appriseit.com/services/splunk/",
	})
}
