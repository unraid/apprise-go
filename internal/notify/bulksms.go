package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const bulkSMSURL = "https://api.bulksms.com/v1/messages"
const bulkSMSBatchSize = 4000

var bulkSMSRoutes = map[string]struct{}{
	"ECONOMY":  {},
	"STANDARD": {},
	"PREMIUM":  {},
}

var bulkSMSGroupRe = regexp.MustCompile(`(?i)^@?([a-z0-9_-]+)$`)

type BulkSMSTarget struct {
	user     string
	password string
	source   string
	route    string
	unicode  bool
	batch    bool
	targets  []string
	groups   []string
}

func NewBulkSMSTarget(target *ParsedURL) (*BulkSMSTarget, error) {
	user := strings.TrimSpace(target.User)
	password := target.Password
	if user == "" || password == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	source := strings.TrimSpace(target.Query["from"])
	if source != "" {
		normalized, ok := normalizePhone(source)
		if !ok {
			return nil, fmt.Errorf("invalid source")
		}
		source = "+" + normalized
	}

	route := "STANDARD"
	if raw := strings.TrimSpace(target.Query["route"]); raw != "" {
		route = strings.ToUpper(raw)
	}
	if _, ok := bulkSMSRoutes[route]; !ok {
		return nil, fmt.Errorf("invalid route")
	}

	unicode := parseBoolWithDefault(target.Query["unicode"], true)
	batch := parseBoolWithDefault(target.Query["batch"], false)

	entries := []string{}
	if target.Host != "" {
		entries = append(entries, target.Host)
	}
	entries = append(entries, splitPath(target.Path)...)
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	targets := []string{}
	groups := []string{}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if normalized, ok := normalizePhone(entry); ok {
			targets = append(targets, "+"+normalized)
			continue
		}
		if group, ok := parseBulkSMSGroup(entry); ok {
			groups = append(groups, group)
		}
	}

	return &BulkSMSTarget{
		user:     user,
		password: password,
		source:   source,
		route:    route,
		unicode:  unicode,
		batch:    batch,
		targets:  targets,
		groups:   groups,
	}, nil
}

func (b *BulkSMSTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(b.targets) == 0 && len(b.groups) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)

	var toValue any
	if len(b.targets) > 0 {
		if b.batch {
			toValue = b.targets[:minInt(len(b.targets), bulkSMSBatchSize)]
		} else {
			toValue = b.targets[0]
		}
	} else {
		toValue = map[string]string{"type": "GROUP", "name": b.groups[0]}
	}

	payload := b.buildPayload(message, toValue)
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    bulkSMSURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Content-Type":  "application/json",
			"Authorization": basicAuthHeader(b.user, b.password),
		},
		Body: string(data),
	}, nil
}

func (b *BulkSMSTarget) Send(body, title string, notifyType NotifyType) error {
	if len(b.targets) == 0 && len(b.groups) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)

	items := []any{}
	if b.batch {
		for index := 0; index < len(b.targets); index += bulkSMSBatchSize {
			end := index + bulkSMSBatchSize
			if end > len(b.targets) {
				end = len(b.targets)
			}
			items = append(items, b.targets[index:end])
		}
	} else {
		for _, target := range b.targets {
			items = append(items, target)
		}
	}

	for _, group := range b.groups {
		items = append(items, map[string]string{"type": "GROUP", "name": group})
	}

	for _, item := range items {
		payload := b.buildPayload(message, item)
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		spec := RequestSpec{
			Method: "POST",
			URL:    bulkSMSURL,
			Headers: map[string]string{
				"User-Agent":    "Apprise",
				"Accept":        "*/*",
				"Content-Type":  "application/json",
				"Authorization": basicAuthHeader(b.user, b.password),
			},
			Body: string(data),
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (b *BulkSMSTarget) buildPayload(message string, toValue any) map[string]any {
	encoding := "TEXT"
	if b.unicode {
		encoding = "UNICODE"
	}

	payload := map[string]any{
		"to":              toValue,
		"body":            message,
		"routingGroup":    b.route,
		"encoding":        encoding,
		"deliveryReports": "ERRORS",
	}
	if b.source != "" {
		payload["from"] = b.source
	}
	return payload
}

func parseBulkSMSGroup(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	match := bulkSMSGroupRe.FindStringSubmatch(trimmed)
	if match == nil {
		return "", false
	}
	if !strings.HasPrefix(trimmed, "@") && isAllDigits(trimmed) {
		return "", false
	}
	return match[1], true
}

func isAllDigits(raw string) bool {
	for _, r := range raw {
		if r < '0' || r > '9' {
			return false
		}
	}
	return raw != ""
}

func init() {
	RegisterSchemaEntryOrdered(24, SchemaEntry{
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
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
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
					"default":  "STANDARD",
					"map_to":   "route",
					"name":     "Route Group",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"ECONOMY", "STANDARD", "PREMIUM"},
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
				"unicode": map[string]any{
					"default":  true,
					"map_to":   "unicode",
					"name":     "Unicode Characters",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			"templates": []string{"{schema}://{user}:{password}@{targets}"},
			"tokens": map[string]any{
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "bulksms",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"bulksms"},
				},
				"target_group": map[string]any{
					"map_to":   "targets",
					"name":     "Target Group",
					"prefix":   "@",
					"private":  false,
					"regex":    []string{"^[A-Z0-9 _-]+$", "i"},
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
					"group":    []string{"target_group", "target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "User Name",
					"private":  false,
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
		"secure_protocols": []string{"bulksms"},
		"service_name":     "BulkSMS",
		"service_url":      "https://bulksms.com/",
		"setup_url":        "https://appriseit.com/services/bulksms/",
	})
}
