package notify

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	mailersendURL = "https://api.mailersend.com/v1/email"

	// MailerSend rejects an empty subject.
	mailersendEmptySubject = "<no subject>"
)

type MailerSendTarget struct {
	apikey    string
	fromEmail string
	targets   []string
	cc        []string
	bcc       []string
	format    string
}

func NewMailerSendTarget(target *ParsedURL) (*MailerSendTarget, error) {
	apikey := strings.TrimSpace(target.User)
	if apikey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	// The sender is the password field at the host domain.
	fromEmail := strings.TrimSpace(target.Password) + "@" + host

	targets := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		targets = append(targets, parseDelimitedList(to)...)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing recipients")
	}

	format := normalizeNotifyFormat(target.Query["format"])
	if format == "" {
		// MailerSend defaults to HTML upstream.
		format = "html"
	}

	return &MailerSendTarget{
		apikey:    apikey,
		fromEmail: fromEmail,
		targets:   targets,
		cc:        parseDelimitedList(target.Query["cc"]),
		bcc:       parseDelimitedList(target.Query["bcc"]),
		format:    format,
	}, nil
}

func (m *MailerSendTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := m.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (m *MailerSendTarget) Send(body, title string, notifyType NotifyType) error {
	specs, err := m.buildRequests(body, title, notifyType)
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

// buildRequests sends one message per recipient, excluding that recipient from
// the copy lists so nobody is addressed twice.
func (m *MailerSendTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	subject := title
	if strings.TrimSpace(subject) == "" {
		subject = mailersendEmptySubject
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Authorization": "Bearer " + m.apikey,
	}

	addresses := func(entries []string, exclude ...string) []any {
		out := []any{}
		for _, entry := range entries {
			if slices.Contains(exclude, entry) {
				continue
			}
			out = append(out, map[string]any{"email": entry})
		}
		return out
	}

	specs := make([]RequestSpec, 0, len(m.targets))
	for _, recipient := range m.targets {
		payload := map[string]any{
			"from":    map[string]any{"email": m.fromEmail},
			"to":      []any{map[string]any{"email": recipient}},
			"subject": subject,
		}

		if m.format == "html" {
			payload["html"] = body
			payload["text"] = htmlToText(body)
		} else {
			payload["text"] = body
			payload["html"] = markdownToHTML(body)
		}

		// A bcc recipient is never also carbon copied.
		if cc := addresses(m.cc, append([]string{recipient}, m.bcc...)...); len(cc) > 0 {
			payload["cc"] = cc
		}
		if bcc := addresses(m.bcc, recipient); len(bcc) > 0 {
			payload["bcc"] = bcc
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     mailersendURL,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}

func init() {
	RegisterSchemaEntryOrdered(151, SchemaEntry{
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
					"default":  "html",
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
				"reply": map[string]any{
					"map_to":   "reply_to",
					"name":     "Reply To",
					"private":  false,
					"required": false,
					"type":     "string",
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
			"templates": []string{"{schema}://{apikey}:{from_email}", "{schema}://{apikey}:{from_email}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[a-zA-Z0-9._-]+$", "i"},
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
					"default":  "mailersend",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"mailersend"},
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
		"enabled": true,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"mailersend"},
		"service_name":     "MailerSend",
		"service_url":      "https://www.mailersend.com/",
		"setup_url":        "https://appriseit.com/services/mailersend/",
	})
}
