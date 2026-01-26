package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const brevoURL = "https://api.brevo.com/v3/smtp/email"
const brevoDefaultSubject = "<no subject>"

type BrevoTarget struct {
	apiKey    string
	fromEmail string
	targets   []string
	cc        map[string]struct{}
	bcc       map[string]struct{}
	replyTo   string
}

func NewBrevoTarget(target *ParsedURL) (*BrevoTarget, error) {
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

	replyTo := strings.TrimSpace(target.Query["reply"])

	return &BrevoTarget{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		targets:   targets,
		cc:        cc,
		bcc:       bcc,
		replyTo:   replyTo,
	}, nil
}

func (b *BrevoTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(b.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	payload := b.buildPayload(body, title, b.targets[0])
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    brevoURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "application/json",
			"Content-Type": "application/json",
			"api-key":      b.apiKey,
		},
		Body: string(data),
	}, nil
}

func (b *BrevoTarget) Send(body, title string, notifyType NotifyType) error {
	if len(b.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	for _, target := range b.targets {
		payload := b.buildPayload(body, title, target)
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    brevoURL,
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "application/json",
				"Content-Type": "application/json",
				"api-key":      b.apiKey,
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

func (b *BrevoTarget) buildPayload(body, title, target string) map[string]any {
	subject := title
	if strings.TrimSpace(subject) == "" {
		subject = brevoDefaultSubject
	}

	payload := map[string]any{
		"sender": map[string]string{
			"email": b.fromEmail,
		},
		"to": []map[string]string{
			{"email": target},
		},
		"subject":     subject,
		"htmlContent": body,
		"textContent": body,
	}

	cc := filterEmailSet(b.cc, b.bcc, target)
	if len(cc) > 0 {
		payload["cc"] = toEmailObjects(cc)
	}

	bcc := filterEmailSet(b.bcc, nil, target)
	if len(bcc) > 0 {
		payload["bcc"] = toEmailObjects(bcc)
	}

	if b.replyTo != "" && isSimpleEmail(b.replyTo) {
		payload["replyTo"] = map[string]string{
			"email": b.replyTo,
		}
	}

	return payload
}

func init() {
	RegisterSchemaEntryOrdered(116, SchemaEntry{
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
				"reply": map[string]any{
					"map_to":   "reply_to",
					"name":     "Reply To Email",
					"private":  false,
					"required": false,
					"type":     "string",
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
					"default":  "brevo",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"brevo"},
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
		"secure_protocols": []string{"brevo"},
		"service_name":     "Brevo",
		"service_url":      "https://www.brevo.com/",
		"setup_url":        "https://appriseit.com/services/brevo/",
	})
}
