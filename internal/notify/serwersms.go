package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	serwersmsURL = "https://api2.serwersms.pl/messages/send_sms"

	// Files switch the send to the MMS endpoint, which takes form fields
	// rather than the JSON body an SMS carries.
	serwersmsMMSURL = "https://api2.serwersms.pl/messages/send_mms"
)

// serwersmsGroup matches a group target, which carries a leading hash that may
// arrive percent encoded.
var serwersmsGroup = regexp.MustCompile(`(?i)^\s*(?:#|%23)([0-9]+)\s*$`)

type SerwerSMSTarget struct {
	user     string
	password string
	sender   string
	phones   []string
	groups   []string
}

func NewSerwerSMSTarget(target *ParsedURL) (*SerwerSMSTarget, error) {
	user := strings.TrimSpace(target.User)
	if user == "" {
		return nil, fmt.Errorf("missing username")
	}
	password := target.Password
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("missing password")
	}

	// The host is the sender name rather than a server.
	sender := strings.TrimSpace(target.Host)
	if raw := strings.TrimSpace(target.Query["from"]); raw != "" {
		sender = raw
	}
	if sender == "" {
		return nil, fmt.Errorf("missing sender")
	}

	entries := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	var phones, groups []string
	for _, entry := range entries {
		if match := serwersmsGroup.FindStringSubmatch(entry); match != nil {
			groups = append(groups, match[1])
			continue
		}
		if normalized, ok := normalizePhone(entry); ok {
			phones = append(phones, normalized)
		}
	}
	if len(phones) == 0 && len(groups) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &SerwerSMSTarget{
		user:     user,
		password: password,
		sender:   sender,
		phones:   phones,
		groups:   groups,
	}, nil
}

func (s *SerwerSMSTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := s.buildRequests(body, title, notifyType, nil)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (s *SerwerSMSTarget) Send(body, title string, notifyType NotifyType) error {
	return s.SendWithAttachments(body, title, notifyType, nil)
}

func (s *SerwerSMSTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	specs, err := s.buildRequests(body, title, notifyType, attachments)
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

// buildRequests sends one message per phone number and one per group.
func (s *SerwerSMSTarget) buildRequests(body, title string, notifyType NotifyType, attachments []Attachment) ([]RequestSpec, error) {
	_ = notifyType

	headers := map[string]string{
		"User-Agent": "Apprise",
	}
	if len(attachments) == 0 {
		headers["Content-Type"] = "application/json"
	}

	// The target field is appended rather than merged from a map, because a
	// multipart body sends these as parts in this order.
	build := func(targetField, targetValue string) (RequestSpec, error) {
		fields := formFields{}
		fields.Set("username", s.user)
		fields.Set("password", s.password)
		fields.Set("text", mergeTitleBody(title, body))
		fields.Set("sender", s.sender)
		fields.Set(targetField, targetValue)

		if len(attachments) > 0 {
			// The same fields travel as form parts, each file repeating the
			// field name file.
			requestBody, contentType, err := indexedFileAttachmentBody(
				fields,
				func(int) string { return "file" },
				attachments, true)
			if err != nil {
				return RequestSpec{}, err
			}

			mmsHeaders := map[string]string{}
			for key, value := range headers {
				mmsHeaders[key] = value
			}
			mmsHeaders["Content-Type"] = contentType

			return RequestSpec{
				Method:  "POST",
				URL:     serwersmsMMSURL,
				Headers: mmsHeaders,
				Body:    requestBody,
			}, nil
		}

		payload := map[string]any{}
		for i, name := range fields.names {
			payload[name] = fields.values[i]
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return RequestSpec{}, err
		}

		return RequestSpec{
			Method:  "POST",
			URL:     serwersmsURL,
			Headers: headers,
			Body:    string(data),
		}, nil
	}

	specs := make([]RequestSpec, 0, len(s.phones)+len(s.groups))
	for _, phone := range s.phones {
		spec, err := build("phone", "+"+phone)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	for _, group := range s.groups {
		spec, err := build("group_id", group)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

func init() {
	RegisterSchemaEntryOrdered(157, SchemaEntry{
		"attachment_support": true,
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
				"from": map[string]any{
					"alias_of": "sender",
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
				"sender": map[string]any{
					"alias_of": "sender",
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
			"templates": []string{"{schema}://{user}:{password}@{sender}/{targets}", "{schema}://{user}:{password}@{sender}"},
			"tokens": map[string]any{
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "serwersms",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"serwersms"},
				},
				"sender": map[string]any{
					"map_to":   "sender",
					"name":     "Sender Name",
					"private":  false,
					"regex":    []string{"^[a-z0-9][a-z0-9 _-]{0,10}$", "i"},
					"required": true,
					"type":     "string",
				},
				"target_group": map[string]any{
					"map_to":   "targets",
					"name":     "Target Group ID",
					"prefix":   "#",
					"private":  false,
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
					"required": false,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Username",
					"private":  false,
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
		"protocols":        nil,
		"secure_protocols": []string{"serwersms"},
		"service_name":     "SerwerSMS",
		"service_url":      "https://serwersms.pl",
		"setup_url":        "https://appriseit.com/services/serwersms/",
	})
}
