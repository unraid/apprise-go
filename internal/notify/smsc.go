package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	smscURL = "https://smsc.ru/sys/send.php"

	// The API returns JSON when format 3 is requested.
	smscFormatJSON = "3"
)

type SMSCTarget struct {
	user     string
	password string
	targets  []string
	sender   string
	translit bool
}

func NewSMSCTarget(target *ParsedURL) (*SMSCTarget, error) {
	user := strings.TrimSpace(target.User)
	if user == "" {
		return nil, fmt.Errorf("missing login")
	}
	password := target.Password
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("missing password")
	}

	// The host is the first recipient, not a server name.
	entries := append([]string{strings.TrimSpace(target.Host)}, splitPath(target.Path)...)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	targets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if normalized, ok := normalizePhone(entry); ok {
			targets = append(targets, normalized)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &SMSCTarget{
		user:     user,
		password: password,
		targets:  targets,
		sender:   strings.TrimSpace(target.Query["sender"]),
		translit: parseBool(target.Query["translit"], false),
	}, nil
}

func (s *SMSCTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return s.buildRequest(body, title, notifyType, nil)
}

func (s *SMSCTarget) buildRequest(body, title string, notifyType NotifyType, attachments []Attachment) (RequestSpec, error) {
	_ = notifyType

	// One request carries every recipient, comma separated.
	values := url.Values{}
	values.Set("login", s.user)
	values.Set("psw", s.password)
	values.Set("phones", strings.Join(s.targets, ","))
	values.Set("fmt", smscFormatJSON)
	if s.sender != "" {
		values.Set("sender", s.sender)
	}
	if s.translit {
		values.Set("translit", "1")
	}
	values.Set("mes", mergeTitleBody(title, body))

	requestBody := values.Encode()
	contentType := "application/x-www-form-urlencoded"
	if len(attachments) > 0 {
		// Files are numbered mes1, mes2 and turn the body multipart.
		var err error
		// The mms flag is what tells SMSC this is a multimedia message, and
		// the files are numbered from zero rather than one.
		mmsValues := url.Values{}
		for key, entries := range values {
			mmsValues[key] = entries
		}
		mmsValues.Set("mms", "1")

		requestBody, contentType, err = indexedFileAttachmentBody(
			mmsValues,
			func(index int) string { return fmt.Sprintf("mes%d", index) },
			attachments, true)
		if err != nil {
			return RequestSpec{}, err
		}
	}

	return RequestSpec{
		Method: "POST",
		URL:    smscURL,
		Headers: map[string]string{
			// SMSC is one of the few endpoints upstream posts to without a
			// User-Agent.
			"Accept":       "*/*",
			"Content-Type": contentType,
		},
		Body: requestBody,
	}, nil
}

func (s *SMSCTarget) Send(body, title string, notifyType NotifyType) error {
	return s.SendWithAttachments(body, title, notifyType, nil)
}

func (s *SMSCTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	spec, err := s.buildRequest(body, title, notifyType, attachments)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(153, SchemaEntry{
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
					"map_to":   "sender",
					"name":     "Sender ID",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"translit": map[string]any{
					"default":  false,
					"map_to":   "translit",
					"name":     "Transliterate",
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
					"default":  "smsc",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"smsc"},
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
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Login",
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
		"secure_protocols": []string{"smsc"},
		"service_name":     "SMSC",
		"service_url":      "https://smsc.ru/",
		"setup_url":        "https://appriseit.com/services/smsc/",
	})
}
