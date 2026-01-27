package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const smtp2goURL = "https://api.smtp2go.com/v3/email/send"
const smtp2goBatchSize = 100

var smtp2goEmailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type SMTP2GoTarget struct {
	apiKey   string
	fromAddr string
	fromName string
	targets  []emailEntry
	cc       []emailEntry
	bcc      []string
	headers  map[string]string
	batch    bool
}

type emailEntry struct {
	name  string
	email string
}

func NewSMTP2GoTarget(target *ParsedURL) (*SMTP2GoTarget, error) {
	user := strings.TrimSpace(target.User)
	host := strings.TrimSpace(target.Host)
	if user == "" || host == "" {
		return nil, fmt.Errorf("missing sender")
	}
	fromAddr := user + "@" + host
	if !isSimpleEmail(fromAddr) {
		return nil, fmt.Errorf("invalid sender")
	}

	pathEntries := splitPath(target.Path)
	if len(pathEntries) == 0 {
		return nil, fmt.Errorf("missing apikey")
	}
	apiKey := strings.TrimSpace(pathEntries[0])
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	targets := []emailEntry{}
	for _, entry := range pathEntries[1:] {
		if parsed, ok := parseEmailEntry(entry); ok {
			targets = append(targets, parsed)
		}
	}
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			if parsed, ok := parseEmailEntry(entry); ok {
				targets = append(targets, parsed)
			}
		}
	}

	fromName := strings.TrimSpace(target.Query["name"])

	cc := []emailEntry{}
	if ccValue, ok := target.Query["cc"]; ok && ccValue != "" {
		for _, entry := range parseDelimitedList(ccValue) {
			if parsed, ok := parseEmailEntry(entry); ok {
				cc = append(cc, parsed)
			}
		}
	}

	bcc := []string{}
	if bccValue, ok := target.Query["bcc"]; ok && bccValue != "" {
		for _, entry := range parseDelimitedList(bccValue) {
			entry = strings.TrimSpace(entry)
			if isSimpleEmail(entry) {
				bcc = append(bcc, entry)
			}
		}
	}

	headers := map[string]string{}
	for key, value := range target.QueryAdd {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		headers[key] = value
	}

	batch := parseBoolWithDefault(target.Query["batch"], false)

	if len(targets) == 0 {
		targets = []emailEntry{{name: fromName, email: fromAddr}}
	}

	return &SMTP2GoTarget{
		apiKey:   apiKey,
		fromAddr: fromAddr,
		fromName: fromName,
		targets:  targets,
		cc:       cc,
		bcc:      bcc,
		headers:  headers,
		batch:    batch,
	}, nil
}

func (s *SMTP2GoTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(s.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	batchSize := 1
	if s.batch {
		batchSize = smtp2goBatchSize
	}

	payload := s.buildPayload(body, title, s.targets[:minInt(len(s.targets), batchSize)])
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    smtp2goURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "application/json",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (s *SMTP2GoTarget) Send(body, title string, notifyType NotifyType) error {
	if len(s.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	batchSize := 1
	if s.batch {
		batchSize = smtp2goBatchSize
	}

	for index := 0; index < len(s.targets); index += batchSize {
		end := index + batchSize
		if end > len(s.targets) {
			end = len(s.targets)
		}

		payload := s.buildPayload(body, title, s.targets[index:end])
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    smtp2goURL,
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "application/json",
				"Content-Type": "application/json",
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

func (s *SMTP2GoTarget) buildPayload(body, title string, recipients []emailEntry) map[string]any {
	payload := map[string]any{
		"api_key":   s.apiKey,
		"sender":    formatEmail(s.fromName, s.fromAddr),
		"subject":   title,
		"to":        []string{},
		"html_body": body,
	}

	to := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		to = append(to, formatEmail(recipient.name, recipient.email))
	}
	payload["to"] = to

	if len(s.cc) > 0 {
		cc := make([]string, 0, len(s.cc))
		for _, recipient := range s.cc {
			cc = append(cc, formatEmail(recipient.name, recipient.email))
		}
		payload["cc"] = cc
	}

	if len(s.bcc) > 0 {
		payload["bcc"] = s.bcc
	}

	if len(s.headers) > 0 {
		customHeaders := make([]map[string]string, 0, len(s.headers))
		for key, value := range s.headers {
			customHeaders = append(customHeaders, map[string]string{
				"header": key,
				"value":  value,
			})
		}
		payload["custom_headers"] = customHeaders
	}

	return payload
}

func parseEmailEntry(raw string) (emailEntry, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return emailEntry{}, false
	}

	name := ""
	email := trimmed
	if parts := strings.SplitN(trimmed, ":", 2); len(parts) == 2 {
		name = strings.TrimSpace(parts[0])
		email = strings.TrimSpace(parts[1])
	}

	if !isSimpleEmail(email) {
		return emailEntry{}, false
	}

	return emailEntry{name: name, email: email}, true
}

func formatEmail(name, email string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", quoteEmailName(name), email)
}

func quoteEmailName(name string) string {
	if name == "" {
		return ""
	}
	if !strings.ContainsAny(name, "()<>\",;:@[]\r\n\t") {
		return name
	}
	escaped := strings.ReplaceAll(name, "\"", "\\\"")
	return "\"" + escaped + "\""
}

func isSimpleEmail(value string) bool {
	return smtp2goEmailRegex.MatchString(value)
}

func init() {
	RegisterSchemaEntryOrdered(18, SchemaEntry{
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
				"bcc": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "bcc",
					"name":     "Blind Carbon Copy",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"cc": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "cc",
					"name":     "Carbon Copy",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
					"default":  "html",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"name": map[string]any{
					"map_to":   "from_name",
					"name":     "From Name",
					"private":  false,
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
					"name":     "Email Header",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{user}@{host}:{apikey}/", "{schema}://{user}@{host}:{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Domain",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "smtp2go",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"smtp2go"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []any{},
					"map_to":   "targets",
					"name":     "Target Emails",
					"private":  false,
					"required": false,
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
		"secure_protocols": []string{"smtp2go"},
		"service_name":     "SMTP2Go",
		"service_url":      "https://www.smtp2go.com/",
		"setup_url":        "https://appriseit.com/services/smtp2go/",
	})
}
