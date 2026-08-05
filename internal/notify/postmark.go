package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	postmarkURL = "https://api.postmarkapp.com/email"

	// Postmark rejects an empty subject.
	postmarkEmptySubject = "<no subject>"
)

type PostmarkTarget struct {
	apikey    string
	fromEmail string
	fromName  string
	targets   []string
	cc        []string
	bcc       []string
	format    string
}

func NewPostmarkTarget(target *ParsedURL) (*PostmarkTarget, error) {
	apikey := strings.TrimSpace(target.Query["apikey"])
	if apikey == "" {
		apikey = strings.TrimSpace(target.User)
	}
	if apikey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	host := strings.TrimSpace(target.Host)
	targets := splitPath(target.Path)

	// ?from= gives the sender outright and frees the host to be a recipient;
	// otherwise the sender is assembled from the credentials and the host.
	fromEmail := strings.TrimSpace(target.Query["from"])
	if fromEmail != "" {
		if host != "" {
			targets = append([]string{host}, targets...)
		}
	} else {
		user := strings.TrimSpace(target.Password)
		if user == "" {
			user = strings.TrimSpace(target.User)
		}
		fromEmail = user + "@" + host
	}

	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		targets = append(targets, parseDelimitedList(to)...)
	}
	// Upstream validates recipients and drops anything that is not an email,
	// which matters because the host lands in this list when ?from= is used.
	valid := make([]string, 0, len(targets))
	for _, entry := range targets {
		if entry = strings.TrimSpace(entry); strings.Contains(entry, "@") {
			valid = append(valid, entry)
		}
	}
	// Upstream defaults the recipient to the sender when the URL names none.
	// This applies only when nothing was specified: recipients that were given
	// and turned out to be invalid are dropped and leave the list empty, which
	// is a URL that delivers to nobody rather than one that delivers to self.
	if len(targets) == 0 {
		targets = []string{fromEmail}
	} else {
		targets = valid
	}

	format := normalizeNotifyFormat(target.Query["format"])
	if format == "" {
		// Postmark defaults to HTML upstream.
		format = "html"
	}

	return &PostmarkTarget{
		apikey:    apikey,
		fromEmail: fromEmail,
		fromName:  strings.TrimSpace(target.Query["name"]),
		targets:   targets,
		cc:        parseDelimitedList(target.Query["cc"]),
		bcc:       parseDelimitedList(target.Query["bcc"]),
		format:    format,
	}, nil
}

func (p *PostmarkTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := p.buildRequests(body, title, notifyType, nil)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (p *PostmarkTarget) Send(body, title string, notifyType NotifyType) error {
	return p.SendWithAttachments(body, title, notifyType, nil)
}

func (p *PostmarkTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	// Upstream keeps going after a failed target; see sendOutcome.
	var outcome sendOutcome
	specs, err := p.buildRequests(body, title, notifyType, attachments)
	if err != nil {
		return err
	}

	for _, spec := range specs {
		outcome.record(SendRequest(spec))
	}
	if err := outcome.err(); err != nil {
		return err
	}

	return nil
}

// buildRequests sends one message per recipient.
func (p *PostmarkTarget) buildRequests(body, title string, notifyType NotifyType, attachments []Attachment) ([]RequestSpec, error) {
	_ = notifyType

	subject := title
	if strings.TrimSpace(subject) == "" {
		subject = postmarkEmptySubject
	}

	headers := map[string]string{
		"User-Agent":              "Apprise",
		"Content-Type":            "application/json",
		"Accept":                  "application/json",
		"X-Postmark-Server-Token": p.apikey,
	}

	specs := make([]RequestSpec, 0, len(p.targets))
	for _, recipient := range p.targets {
		payload := map[string]any{
			"From":    formatMIMEAddress(p.fromName, p.fromEmail),
			"Subject": subject,
			"To":      recipient,
		}
		if p.format == "html" {
			payload["HtmlBody"] = body
		} else {
			payload["TextBody"] = body
		}
		if len(p.cc) > 0 {
			payload["Cc"] = strings.Join(p.cc, ",")
		}
		if len(p.bcc) > 0 {
			payload["Bcc"] = strings.Join(p.bcc, ",")
		}
		if len(attachments) > 0 {
			payload["Attachments"] = attachmentsPostmarkStyle(attachments)
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     postmarkURL,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}

func init() {
	RegisterSchemaEntryOrdered(150, SchemaEntry{
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
				"name": map[string]any{
					"map_to":   "from_name",
					"name":     "From Name",
					"private":  false,
					"required": false,
					"type":     "string",
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
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "reply_to",
					"name":     "Reply To Email",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
					"default":  "postmark",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"postmark"},
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
		"protocols":        nil,
		"secure_protocols": []string{"postmark"},
		"service_name":     "Postmark",
		"service_url":      "https://postmarkapp.com/",
		"setup_url":        "https://appriseit.com/services/postmark/",
	})
}
