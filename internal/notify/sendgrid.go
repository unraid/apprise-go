package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const sendgridURL = "https://api.sendgrid.com/v3/mail/send"
const sendgridDefaultSubject = "<no subject>"

type SendGridTarget struct {
	apiKey       string
	fromEmail    string
	targets      []string
	cc           map[string]struct{}
	bcc          map[string]struct{}
	templateID   string
	templateData map[string]string
}

func NewSendGridTarget(target *ParsedURL) (*SendGridTarget, error) {
	apiKey := strings.TrimSpace(target.User)
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	local := strings.TrimSpace(target.Password)
	host := strings.TrimSpace(target.Host)
	if local == "" || host == "" {
		return nil, fmt.Errorf("missing from email")
	}
	fromEmail := local + "@" + host
	if !isSimpleEmail(fromEmail) {
		return nil, fmt.Errorf("invalid from email")
	}

	targets := []string{}
	for _, entry := range splitPath(target.Path) {
		entry = strings.TrimSpace(entry)
		if isSimpleEmail(entry) {
			targets = append(targets, entry)
		}
	}
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			entry = strings.TrimSpace(entry)
			if isSimpleEmail(entry) {
				targets = append(targets, entry)
			}
		}
	}
	if len(targets) == 0 {
		targets = append(targets, fromEmail)
	}

	cc := map[string]struct{}{}
	if ccValue, ok := target.Query["cc"]; ok && ccValue != "" {
		for _, entry := range parseDelimitedList(ccValue) {
			entry = strings.TrimSpace(entry)
			if isSimpleEmail(entry) {
				cc[entry] = struct{}{}
			}
		}
	}

	bcc := map[string]struct{}{}
	if bccValue, ok := target.Query["bcc"]; ok && bccValue != "" {
		for _, entry := range parseDelimitedList(bccValue) {
			entry = strings.TrimSpace(entry)
			if isSimpleEmail(entry) {
				bcc[entry] = struct{}{}
			}
		}
	}

	templateData := map[string]string{}
	for key, value := range target.QueryAdd {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		templateData[key] = value
	}

	return &SendGridTarget{
		apiKey:       apiKey,
		fromEmail:    fromEmail,
		targets:      targets,
		cc:           cc,
		bcc:          bcc,
		templateID:   strings.TrimSpace(target.Query["template"]),
		templateData: templateData,
	}, nil
}

func (s *SendGridTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(s.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	payload := s.buildPayload(body, title, s.targets[0])
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    sendgridURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
			"Authorization": fmt.Sprintf(
				"Bearer %s",
				s.apiKey,
			),
		},
		Body: string(data),
	}, nil
}

func (s *SendGridTarget) Send(body, title string, notifyType NotifyType) error {
	if len(s.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	for _, target := range s.targets {
		payload := s.buildPayload(body, title, target)
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    sendgridURL,
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "*/*",
				"Content-Type": "application/json",
				"Authorization": fmt.Sprintf(
					"Bearer %s",
					s.apiKey,
				),
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

func (s *SendGridTarget) buildPayload(body, title, target string) map[string]any {
	subject := title
	if strings.TrimSpace(subject) == "" {
		subject = sendgridDefaultSubject
	}

	payload := map[string]any{
		"personalizations": []any{
			map[string]any{
				"to": []map[string]string{
					{"email": target},
				},
			},
		},
		"from": map[string]string{
			"email": s.fromEmail,
		},
		"subject": subject,
		"content": []map[string]string{
			{
				"type":  "text/html",
				"value": body,
			},
		},
	}

	cc := filterEmailSet(s.cc, s.bcc, target)
	if len(cc) > 0 {
		payload["personalizations"].([]any)[0].(map[string]any)["cc"] = toEmailObjects(cc)
	}

	bcc := filterEmailSet(s.bcc, nil, target)
	if len(bcc) > 0 {
		payload["personalizations"].([]any)[0].(map[string]any)["bcc"] = toEmailObjects(bcc)
	}

	if s.templateID != "" {
		payload["template_id"] = s.templateID
		if len(s.templateData) > 0 {
			payload["personalizations"].([]any)[0].(map[string]any)["dynamic_template_data"] = s.templateData
		}
	}

	return payload
}

func filterEmailSet(source, remove map[string]struct{}, target string) []string {
	filtered := []string{}
	for email := range source {
		if email == target {
			continue
		}
		if remove != nil {
			if _, ok := remove[email]; ok {
				continue
			}
		}
		filtered = append(filtered, email)
	}
	return filtered
}

func toEmailObjects(entries []string) []map[string]string {
	result := make([]map[string]string, 0, len(entries))
	for _, email := range entries {
		result = append(result, map[string]string{"email": email})
	}
	return result
}

func init() {
	RegisterSchemaEntryOrdered(90, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
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
					"map_to":   "template",
					"name":     "Template",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"template_data": map[string]any{
					"map_to":   "template_data",
					"name":     "Template Data",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{apikey}:{from_email}", "{schema}://{apikey}:{from_email}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[A-Z0-9._-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"from_email": map[string]any{
					"map_to":   "from_email",
					"name":     "Source Email",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "sendgrid",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"sendgrid"},
				},
				"target_email": map[string]any{
					"map_to":   "targets",
					"name":     "Target Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_email"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
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
		"secure_protocols": []string{"sendgrid"},
		"service_name":     "SendGrid",
		"service_url":      "https://sendgrid.com",
		"setup_url":        "https://appriseit.com/services/sendgrid/",
	})
}
