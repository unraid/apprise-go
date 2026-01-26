package notify

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const smseaglePath = "/jsonrpc/sms"
const smseagleBatchSize = 10

type SMSEagleTarget struct {
	token    string
	host     string
	port     int
	secure   bool
	batch    bool
	status   bool
	flash    bool
	testMode bool
	priority int
	phones   []string
	groups   []string
	contacts []string
}

func NewSMSEagleTarget(target *ParsedURL) (*SMSEagleTarget, error) {
	if target.Host == "" {
		return nil, fmt.Errorf("missing host")
	}

	token := target.User
	if rawToken, ok := target.Query["token"]; ok && rawToken != "" {
		token = rawToken
	}
	if token == "" || target.Password != "" {
		if token == "" {
			return nil, fmt.Errorf("missing token")
		}
	}

	priority, err := parseSMSEaglePriority(target.Query["priority"])
	if err != nil {
		return nil, err
	}

	batch := parseBool(target.Query["batch"], false)
	status := parseBool(target.Query["status"], false)
	flash := parseBool(target.Query["flash"], false)
	testMode := parseBool(target.Query["test"], false)

	entries := splitPath(target.Path)
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	phones := []string{}
	groups := []string{}
	contacts := []string{}
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if normalized, ok := normalizePhoneWithBounds(trimmed, 9, 14); ok {
			if strings.HasPrefix(trimmed, "+") {
				phones = append(phones, "+"+normalized)
			} else {
				phones = append(phones, normalized)
			}
			continue
		}
		if group := parseSMSEagleGroup(trimmed); group != "" {
			groups = append(groups, group)
			continue
		}
		if contact := parseSMSEagleContact(trimmed); contact != "" {
			contacts = append(contacts, contact)
			continue
		}
	}

	return &SMSEagleTarget{
		token:    token,
		host:     target.Host,
		port:     target.Port,
		secure:   target.Scheme == "smseagles",
		batch:    batch,
		status:   status,
		flash:    flash,
		testMode: testMode,
		priority: priority,
		phones:   phones,
		groups:   groups,
		contacts: contacts,
	}, nil
}

func (s *SMSEagleTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	category, targets := s.pickTargets()
	if category == "" || len(targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	if s.status {
		message = notifyTypeASCII(notifyType) + " " + message
	}

	method, targetKey := smseagleMethod(category)
	value := smseagleJoinTargets(targets, s.batch)

	payload := s.buildPayload(method, targetKey, value, message)
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    s.buildURL(),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (s *SMSEagleTarget) Send(body, title string, notifyType NotifyType) error {
	message := mergeTitleBody(title, body)
	if s.status {
		message = notifyTypeASCII(notifyType) + " " + message
	}

	for _, category := range []string{"phone", "group", "contact"} {
		var targets []string
		switch category {
		case "phone":
			targets = s.phones
		case "group":
			targets = s.groups
		case "contact":
			targets = s.contacts
		}
		if len(targets) == 0 {
			continue
		}

		method, targetKey := smseagleMethod(category)
		batchSize := 1
		if s.batch {
			batchSize = smseagleBatchSize
		}

		for index := 0; index < len(targets); index += batchSize {
			end := index + batchSize
			if end > len(targets) {
				end = len(targets)
			}
			value := strings.Join(targets[index:end], ",")
			payload := s.buildPayload(method, targetKey, value, message)
			data, err := json.Marshal(payload)
			if err != nil {
				return err
			}

			spec := RequestSpec{
				Method: "POST",
				URL:    s.buildURL(),
				Headers: map[string]string{
					"User-Agent":   "Apprise",
					"Accept":       "*/*",
					"Content-Type": "application/json",
				},
				Body: string(data),
			}
			if err := SendRequest(spec); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *SMSEagleTarget) buildURL() string {
	scheme := "http"
	if s.secure {
		scheme = "https"
	}

	host := s.host
	if s.port != 0 {
		host = fmt.Sprintf("%s:%d", host, s.port)
	}

	return scheme + "://" + host + smseaglePath
}

func (s *SMSEagleTarget) buildPayload(method, targetKey, targetValue, message string) map[string]any {
	params := map[string]any{
		targetKey:      targetValue,
		"access_token": s.token,
		"message":      message,
		"highpriority": s.priority,
		"unicode":      1,
		"message_type": "sms",
		"responsetype": "extended",
		"flash":        boolToInt(s.flash),
		"test":         boolToInt(s.testMode),
	}

	return map[string]any{
		"method": method,
		"params": params,
	}
}

func (s *SMSEagleTarget) pickTargets() (string, []string) {
	if len(s.phones) > 0 {
		return "phone", s.phones
	}
	if len(s.groups) > 0 {
		return "group", s.groups
	}
	if len(s.contacts) > 0 {
		return "contact", s.contacts
	}
	return "", nil
}

func smseagleMethod(category string) (string, string) {
	switch category {
	case "group":
		return "sms.send_togroup", "groupname"
	case "contact":
		return "sms.send_tocontact", "contactname"
	default:
		return "sms.send_sms", "to"
	}
}

func smseagleJoinTargets(targets []string, batch bool) string {
	if len(targets) == 0 {
		return ""
	}
	if !batch {
		return targets[0]
	}
	end := smseagleBatchSize
	if end > len(targets) {
		end = len(targets)
	}
	return strings.Join(targets[:end], ",")
}

func parseSMSEaglePriority(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}

	if value, err := strconv.Atoi(trimmed); err == nil {
		if value == 0 || value == 1 {
			return value, nil
		}
		return 0, fmt.Errorf("invalid priority")
	}

	lower := strings.ToLower(trimmed)
	if lower == "+" {
		return 1, nil
	}
	if strings.HasPrefix("normal", lower) {
		return 0, nil
	}
	if strings.HasPrefix("high", lower) {
		return 1, nil
	}

	return 0, fmt.Errorf("invalid priority")
}

func parseSMSEagleGroup(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "#") {
		return ""
	}
	value := strings.TrimSpace(trimmed[1:])
	if value == "" {
		return ""
	}
	if !isAlphaNumDash(value) {
		return ""
	}
	return value
}

func parseSMSEagleContact(raw string) string {
	trimmed := strings.TrimSpace(raw)
	value := strings.TrimPrefix(trimmed, "@")
	if value == "" {
		return ""
	}
	if !isAlphaNumDash(value) {
		return ""
	}
	return value
}

func isAlphaNumDash(raw string) bool {
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func init() {
	RegisterSchemaEntryOrdered(56, SchemaEntry{
		"attachment_support": true,
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
				"flash": map[string]any{
					"default":  false,
					"map_to":   "flash",
					"name":     "Flash",
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
				"priority": map[string]any{
					"default":  0,
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{0, 1},
				},
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"status": map[string]any{
					"default":  false,
					"map_to":   "status",
					"name":     "Show Status",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"test": map[string]any{
					"default":  false,
					"map_to":   "test",
					"name":     "Test Only",
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}@{host}/{targets}", "{schema}://{token}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
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
					"values":   []string{"smseagle", "smseagles"},
				},
				"target_contact": map[string]any{
					"map_to":   "targets",
					"name":     "Target Contact",
					"prefix":   "@",
					"private":  false,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"target_group": map[string]any{
					"map_to":   "targets",
					"name":     "Target Group ID",
					"prefix":   "#",
					"private":  false,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": false,
					"type":     "string",
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
					"group":    []string{"target_contact", "target_group", "target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Access Token",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"smseagle"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"smseagles"},
		"service_name":     "SMS Eagle",
		"service_url":      "https://smseagle.eu",
		"setup_url":        "https://appriseit.com/services/smseagle/",
	})
}
