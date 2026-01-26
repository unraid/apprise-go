package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const resendURL = "https://api.resend.com/emails"
const resendDefaultSubject = "<no subject>"

type ResendTarget struct {
	apiKey    string
	fromEmail string
	fromName  string
	targets   []string
	cc        map[string]struct{}
	bcc       map[string]struct{}
	replyTo   map[string]struct{}
	names     map[string]string
}

func NewResendTarget(target *ParsedURL) (*ResendTarget, error) {
	apiKey := strings.TrimSpace(target.Query["apikey"])
	if apiKey == "" {
		apiKey = strings.TrimSpace(target.User)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	fromAddr := strings.TrimSpace(target.Query["from"])
	if fromAddr == "" {
		local := strings.TrimSpace(target.Password)
		host := strings.TrimSpace(target.Host)
		if local == "" || host == "" {
			return nil, fmt.Errorf("missing from address")
		}
		fromAddr = local + "@" + host
	}
	if !isSimpleEmail(fromAddr) {
		return nil, fmt.Errorf("invalid from address")
	}

	fromName := strings.TrimSpace(target.Query["name"])

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
		targets = append(targets, fromAddr)
	}

	names := map[string]string{}
	if fromName != "" {
		names[fromAddr] = fromName
	}

	cc := map[string]struct{}{}
	if ccValue, ok := target.Query["cc"]; ok && ccValue != "" {
		for _, entry := range parseDelimitedList(ccValue) {
			if parsed, ok := parseEmailEntry(entry); ok {
				cc[parsed.email] = struct{}{}
				if parsed.name != "" {
					names[parsed.email] = parsed.name
				}
			}
		}
	}

	bcc := map[string]struct{}{}
	if bccValue, ok := target.Query["bcc"]; ok && bccValue != "" {
		for _, entry := range parseDelimitedList(bccValue) {
			if parsed, ok := parseEmailEntry(entry); ok {
				bcc[parsed.email] = struct{}{}
				if parsed.name != "" {
					names[parsed.email] = parsed.name
				}
			}
		}
	}

	replyTo := map[string]struct{}{}
	if replyValue, ok := target.Query["reply"]; ok && replyValue != "" {
		for _, entry := range parseDelimitedList(replyValue) {
			if parsed, ok := parseEmailEntry(entry); ok {
				replyTo[parsed.email] = struct{}{}
				if parsed.name != "" {
					names[parsed.email] = parsed.name
				}
			}
		}
	}

	return &ResendTarget{
		apiKey:    apiKey,
		fromEmail: fromAddr,
		fromName:  fromName,
		targets:   targets,
		cc:        cc,
		bcc:       bcc,
		replyTo:   replyTo,
		names:     names,
	}, nil
}

func (r *ResendTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(r.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	payload := r.buildPayload(body, title, r.targets[0])
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    resendURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
			"Authorization": fmt.Sprintf(
				"Bearer %s",
				r.apiKey,
			),
		},
		Body: string(data),
	}, nil
}

func (r *ResendTarget) Send(body, title string, notifyType NotifyType) error {
	if len(r.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	for _, target := range r.targets {
		payload := r.buildPayload(body, title, target)
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    resendURL,
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "*/*",
				"Content-Type": "application/json",
				"Authorization": fmt.Sprintf(
					"Bearer %s",
					r.apiKey,
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

func (r *ResendTarget) buildPayload(body, title, target string) map[string]any {
	subject := title
	if strings.TrimSpace(subject) == "" {
		subject = resendDefaultSubject
	}

	fromValue := formatEmail(r.fromName, r.fromEmail)
	payload := map[string]any{
		"from":    fromValue,
		"subject": subject,
		"html":    body,
		"to":      target,
	}

	cc := filterEmailSet(r.cc, r.bcc, target)
	if len(cc) > 0 {
		payload["cc"] = formatEmailList(cc, r.names)
	}

	bcc := filterEmailSet(r.bcc, nil, target)
	if len(bcc) > 0 {
		payload["bcc"] = bcc
	}

	replyTo := filterEmailSet(r.replyTo, nil, target)
	if len(replyTo) > 0 {
		payload["reply_to"] = formatEmailList(replyTo, r.names)
	}

	return payload
}

func formatEmailList(emails []string, names map[string]string) []string {
	formatted := make([]string, 0, len(emails))
	for _, email := range emails {
		name := names[email]
		formatted = append(formatted, formatEmail(name, email))
	}
	return formatted
}

func init() {
	RegisterSchemaEntryOrdered(76, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "apikey",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"from": map[string]any{
					"map_to":   "from_addr",
					"name":     "from",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"name": map[string]any{
					"map_to":   "from_addr",
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
				"reply": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "reply_to",
					"name":     "Reply To",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{apikey}:{from_addr}", "{schema}://{apikey}:{from_addr}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[A-Z0-9._-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"from_addr": map[string]any{
					"map_to":   "from_addr",
					"name":     "Source Email",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "resend",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"resend"},
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
		"secure_protocols": []string{"resend"},
		"service_name":     "Resend",
		"service_url":      "https://resend.com",
		"setup_url":        "https://appriseit.com/services/resend/",
	})
}
